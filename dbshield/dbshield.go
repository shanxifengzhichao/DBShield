/*
Package dbshield implements the database firewall functionality
*/
package dbshield

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"github.com/boltdb/bolt"
	"github.com/nim4/DBShield/dbshield/config"
	"github.com/nim4/DBShield/dbshield/httpserver"
	"github.com/nim4/DBShield/dbshield/logger"
	"github.com/nim4/DBShield/dbshield/sql"
	"github.com/nim4/DBShield/dbshield/training"
)

//Version of the library
var Version = "1.0.0-beta3"

var configFile string

//SetConfigFile of DBShield
func SetConfigFile(cf string) error {
	configFile = cf
	err := config.ParseConfig(configFile)
	if err != nil {
		return err
	}
	return postConfig()
}

//Check config file and writes it to STDUT
func Check() error {
	confJSON, err := json.MarshalIndent(config.Config, "", "    ")
	fmt.Println(string(confJSON))
	return err
}

//Patterns lists the captured patterns
func Patterns() {
	initModel()
	training.DBConLearning.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(pattern []byte, b *bolt.Bucket) error {
			k, v := b.Cursor().First()
			var context sql.QueryContext
			context.Unmarshal(v)
			fmt.Printf(
				`Pattern:     0x%x
Sample query: %s
-----------------
`,
				k,
				context.Query)

			return nil
		})
	})
}

func postConfig() (err error) {

	config.Config.DB, err = dbNameToStruct(config.Config.DBType)
	if err != nil {
		return err
	}

	if config.Config.ListenPort == 0 {
		config.Config.ListenPort = config.Config.DB.DefaultPort()
	}
	if config.Config.TargetPort == 0 {
		config.Config.TargetPort = config.Config.DB.DefaultPort()
	}

	err = config.Config.DB.SetCertificate(config.Config.TLSCertificate, config.Config.TLSPrivateKey)
	if err != nil {
		return
	}
	return
}

//Start the proxy
func Start() (err error) {
	initModel()
	initLogging()
	initSignal()
	logger.Infof("Config file: %s", configFile)
	logger.Infof("Listening: %s:%v",
		config.Config.ListenIP,
		config.Config.ListenPort)
	logger.Infof("Backend: %s (%s:%v)",
		config.Config.DBType,
		config.Config.TargetIP,
		config.Config.TargetPort)
	logger.Infof("Protect: %v", !config.Config.Learning)

	var listenConn net.Conn
	if config.Config.HTTP {
		logger.Infof("Web interface on https://%s/", config.Config.HTTPAddr)
		go httpserver.Serve()
	}
	serverAddr, err := net.ResolveTCPAddr("tcp", config.Config.TargetIP+":"+strconv.Itoa(int(config.Config.TargetPort)))
	if err != nil {
		return
	}
	l, err := net.Listen("tcp", config.Config.ListenIP+":"+strconv.Itoa(int(config.Config.ListenPort)))
	if err != nil {
		return
	}
	// Close the listener when the application closes.
	defer l.Close()

	for {
		// Listen for an incoming connection.
		listenConn, err = l.Accept()
		if err != nil {
			logger.Warningf("Error accepting connection: %v", err)
			continue
		}
		go handleClient(listenConn, serverAddr)
	}
}
