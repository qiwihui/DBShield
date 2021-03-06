package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	// mysql driver
	_ "github.com/go-sql-driver/mysql"
	"github.com/qiwihui/DBShield/dbshield/db"
	"github.com/qiwihui/DBShield/dbshield/logger"
	"github.com/qiwihui/DBShield/dbshield/utils"
	"github.com/spf13/viper"
)

type mask struct {
	MatchExp         *regexp.Regexp
	ReplaceExp       []byte
	PaddingCharacter []byte
}

//Configurations structure to hold user configurations
type Configurations struct {
	Learning    bool
	CheckUser   bool
	CheckSource bool

	LogLevel uint
	LogPath  string

	DBType string
	DB     uint `json:"-"`

	ListenIP   string
	ListenPort uint

	TargetIP   string
	TargetPort uint

	TLSPrivateKey  string
	TLSCertificate string

	HTTP         bool
	HTTPSSL      bool
	HTTPAddr     string
	HTTPPassword string

	Action     string
	ActionFunc func() error `json:"-"`

	Timeout time.Duration

	SyncInterval time.Duration
	//Key-> database.table.column
	//Masks map[string]mask

	localDbID        string
	LocalDbms        string
	DBDir            string
	LocalDB          db.BASE
	LocalDbDsn       string
	LocalQueryRecord bool
}

//Config holds current configs
var Config Configurations

func strConfig(key string) (ret string, err error) {
	if viper.IsSet(key) {
		ret = viper.GetString(key)
		return
	}
	err = fmt.Errorf("Invalid '%s' cofiguration", key)
	return
}

func strConfigDefualt(key, defaultValue string) (ret string) {
	if viper.IsSet(key) {
		ret = viper.GetString(key)
		return
	}
	logger.Infof("'%s' not configured, assuming: %s", key, defaultValue)
	ret = defaultValue
	return
}

func intConfig(key string, defaultValue, min uint) (ret uint, err error) {
	if viper.IsSet(key) {
		tmp := viper.GetInt(key)
		if tmp < 0 {
			err = fmt.Errorf("Invalid '%s' cofiguration: %v\n", key, tmp)
			return
		}
		ret = uint(tmp)
		if ret < min {
			err = fmt.Errorf("Invalid '%s' cofiguration: %v\n", key, ret)
			return
		}
		return
	}
	logger.Infof("'%s' not configured, assuming: %s", key, defaultValue)
	ret = defaultValue
	return
}

func configGeneral() (err error) {
	if viper.IsSet("mode") {
		switch viper.GetString("mode") {
		case "protect":
			Config.Learning = false
		case "learning":
			Config.Learning = true
		default:
			return errors.New("Invalid 'mode' cofiguration: " + viper.GetString("mode"))
		}
	} else {
		logger.Infof("'mode' not configured, assuming: learning")
		Config.Learning = true
	}

	Config.ListenPort, err = intConfig("listenPort", 0, 0)
	if err != nil {
		return err
	}

	Config.TargetPort, err = intConfig("targetPort", 0, 0)
	if err != nil {
		return err
	}
	Config.TargetIP, err = strConfig("targetIP")
	if err != nil {
		return err
	}

	//String values
	Config.TLSPrivateKey, err = strConfig("tlsPrivateKey")
	if err != nil {
		return err
	}

	Config.TLSCertificate, err = strConfig("tlsCertificate")
	if err != nil {
		return err
	}

	Config.DBType = strConfigDefualt("dbms", "mysql")

	Config.ListenIP = strConfigDefualt("listenIP", "0.0.0.0")

	if timeout := viper.GetString("timeout"); timeout != "" {
		Config.Timeout, err = time.ParseDuration(timeout)
		if err != nil {
			return err
		}
	} else {
		Config.Timeout = 5 * time.Second
	}

	if syn := viper.GetString("syncInterval"); syn != "" {
		Config.SyncInterval, err = time.ParseDuration(syn)
		if err != nil {
			return err
		}
	} else {
		Config.SyncInterval = 5 * time.Second
	}

	return nil
}

func configProtect() error {
	if viper.IsSet("action") {
		Config.Action = viper.GetString("action")
		switch Config.Action {
		case "drop": //Close the connection
			Config.ActionFunc = utils.ActionDrop
		case "pass": //Pass the query to server
			Config.ActionFunc = nil
		default:
			return errors.New("Invalid 'action' cofiguration: " + Config.Action)
		}
	} else {
		logger.Infof("'action' not configured, assuming: drop")
		Config.ActionFunc = utils.ActionDrop
	}

	if viper.IsSet("additionalChecks") {
		for _, check := range strings.Split(viper.GetString("additionalChecks"), ",") {
			switch check {
			case "user":
				Config.CheckUser = true
			case "source":
				Config.CheckSource = true
			default:
				return errors.New("Invalid 'additionalChecks' cofiguration: " + check)
			}
		}
	}
	return nil
}

func configLog() error {
	var err error
	Config.LogPath = strConfigDefualt("logPath", "stderr")
	Config.LogLevel, err = intConfig("logLevel", 3, 0)
	return err
}

func configHTTP() error {
	Config.HTTP = viper.GetBool("http")
	if Config.HTTP {
		Config.HTTPPassword = viper.GetString("httpPassword")
		httpIP := strConfigDefualt("httpIP", "127.0.0.1")
		httpPort, err := intConfig("httpPort", 8070, 1)
		if err != nil {
			return err
		}
		Config.HTTPSSL = viper.GetBool("httpSSL")
		Config.HTTPAddr = fmt.Sprintf("%s:%d", httpIP, httpPort)
	}
	return nil
}

func configLocalDb() error {
	var err error

	Config.localDbID, err = strConfig("localDbID")
	if err != nil {
		return err
	}

	Config.LocalDbms, err = strConfig("localDbms")
	if err != nil {
		return err
	}
	dbName := strings.ToLower(Config.LocalDbms)
	Config.LocalDB = db.GenerateLocalDB(dbName, Config.localDbID)
	if Config.LocalDB == nil {
		return errors.New("err")
	}
	var dbPath string
	logger.Infof("DBConfig: %s", dbName)
	switch dbName {
	case "mysql":
		Config.LocalDbDsn, err = strConfig("localDbDsn")
		if err != nil {
			return err
		}
		dbPath = Config.LocalDbDsn

	case "boltdb":
		Config.DBDir = strConfigDefualt("dbDir", os.TempDir()+"/model")
		err = os.MkdirAll(Config.DBDir, 0740) //Make dbDir, just in case its not there
		if err != nil {
			return err
		}
		dbPath = path.Join(Config.DBDir, Config.TargetIP+"_"+Config.DBType) + ".db"
	default:
		err = fmt.Errorf("Invalid 'localDbms' cofiguration")
		return err
	}
	logger.Infof("LocalDBConfig: %s => %s", dbName, dbPath)
	err = Config.LocalDB.InitialDB(dbPath, Config.SyncInterval, Config.Timeout)
	if err != nil {
		return err
	}

	Config.LocalQueryRecord = viper.GetBool("localQueryRecord")
	return nil
}

//ParseConfig and return error if its not valid
func ParseConfig(configFile string) error {
	Config = Configurations{} // Reset configs
	viper.SetConfigFile(configFile)
	err := viper.ReadInConfig() // Read the config file
	if err != nil {
		return fmt.Errorf("Fatal error - config file: %s \n", err)
	}
	err = configGeneral()
	if err != nil {
		return err
	}

	if !Config.Learning {
		err = configProtect()
		if err != nil {
			return err
		}
	}

	err = configLog()
	if err != nil {
		return err
	}

	err = configHTTP()
	if err != nil {
		return err
	}

	err = configLocalDb()
	if err != nil {
		return err
	}
	return nil
}
