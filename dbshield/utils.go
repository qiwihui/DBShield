package dbshield

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/qiwihui/DBShield/dbshield/config"
	"github.com/qiwihui/DBShield/dbshield/dbms"
	"github.com/qiwihui/DBShield/dbshield/logger"
	"github.com/qiwihui/DBShield/dbshield/utils"
)

const (
	mysql = iota
	mssql
	postgres
	db2
	oracle
)

func closeHandlers() {

	//TODO NEED to verify
	if config.Config.LocalDB != nil {
		config.Config.LocalDB.UpdateState()
		config.Config.LocalDB.SyncAndClose()
	}
	if logger.Output != nil {
		logger.Output.Close()
	}
}

//catching Interrupts
func signalHandler() {
	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt)
	<-term
	logger.Info("Shutting down...")
	//Closing open handler politely
	closeHandlers()
}

//initLogging redirect log output to file/stdout/stderr
func initLogging() {
	err := logger.Init(config.Config.LogPath, config.Config.LogLevel)
	if err != nil {
		panic(err)
	}
}

//maps database name to corresponding struct
func dbNameToStruct(db string) (d uint, err error) {
	switch strings.ToLower(db) {
	case "db2":
		d = db2
	case "mssql":
		d = mssql
	case "mysql", "mariadb":
		d = mysql
	case "oracle":
		d = oracle
	case "postgres":
		d = postgres
	default:
		err = fmt.Errorf("Unknown DBMS: %s", db)
	}
	return
}

//generateDBMS instantiate a new instance of DBMS
func generateDBMS() (utils.DBMS, func(io.Reader) ([]byte, error)) {
	switch config.Config.DB {
	case mssql:
		return new(dbms.MSSQL), dbms.MSSQLReadPacket
	case mysql:
		return new(dbms.MySQL), dbms.MySQLReadPacket
	case postgres:
		return new(dbms.Postgres), dbms.ReadPacket //TODO: implement explicit reader
	case oracle:
		return new(dbms.Oracle), dbms.ReadPacket //TODO: implement explicit reader
	case db2:
		return new(dbms.DB2), dbms.ReadPacket //TODO: implement explicit reader
	default:
		return nil, nil
	}
}

func handleClient(listenConn net.Conn, serverAddr *net.TCPAddr) error {
	d, reader := generateDBMS()
	// delay
	// tcpConn := listenConn.(*net.TCPConn)
	// tcpConn.SetNoDelay(false)
	// // tcpConn.SetKeepAlive(true)
	// listenConn = tcpConn

	logger.Debugf("Connected from: %s", listenConn.RemoteAddr())
	serverConn, err := net.DialTCP("tcp", nil, serverAddr)
	if err != nil {
		logger.Warning(err)
		listenConn.Close()
		return err
	}
	// serverConn.SetNoDelay(false)
	// serverConn.SetKeepAlive(true)
	// if err := SetConnTimeout(listenConn); err != nil {
	// 	return err
	// }
	// if err := SetConnTimeout(serverConn); err != nil {
	// 	return err
	// }

	if config.Config.Timeout > 0 {
		tcpCli := listenConn.(*net.TCPConn)
		tcpCli.SetNoDelay(false)
		tcpCli.SetKeepAlive(true)
		serverConn.SetNoDelay(false)
		serverConn.SetKeepAlive(true)
	}

	logger.Debugf("Connected to: %s", serverConn.RemoteAddr())
	d.SetSockets(listenConn, serverConn)
	d.SetCertificate(config.Config.TLSCertificate, config.Config.TLSPrivateKey)
	d.SetReader(reader)
	err = d.Handler()
	if err != nil {
		logger.Warning(err)
	}
	return err
}

// SetConnTimeout for connection
func SetConnTimeout(conn net.Conn) error {
	if config.Config.Timeout > 0 {
		if err := conn.SetDeadline(time.Now().Add(config.Config.Timeout)); err != nil {
			return err
		}
	}
	return nil
}
