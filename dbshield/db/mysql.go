package db

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	// mysql orm
	"github.com/astaxie/beego/orm"
	// mysql driver
	_ "github.com/go-sql-driver/mysql"
	"github.com/qiwihui/DBShield/dbshield/logger"
	"github.com/qiwihui/DBShield/dbshield/sql"
)

//MySQL local db
type MySQL struct {
	name string
}

//QueryAction query and action
type QueryAction struct {
	ID     int       `orm:"column(id)"`
	Query  string    `orm:"column(query);null;type(text)"`
	User   string    `orm:"column(user);null;size(128)"`
	Client string    `orm:"column(client);null;size(128)"`
	Db     string    `orm:"column(db);null;size(128)"`
	Time   time.Time `orm:"column(time);type(datetime);size(6)"`
	Action string    `orm:"column(action);size(32)"`
}

//Pattern record trainging set
type Pattern struct {
	ID    int    `orm:"column(id)"`
	Key   string `orm:"column(key);null;type(text)"`
	Value string `orm:"column(value);null;type(text)"`
}

//Abnormal record abnormal set
type Abnormal struct {
	ID int `orm:"column(id)"`
	// Key   string `orm:"column(key);type(text)"`
	Value string `orm:"column(value);type(text)"`
}

//State record abnormal set
type State struct {
	ID              int    `orm:"column(id)"`
	Key             string `orm:"column(key);size(5)"`
	QueryCounter    uint64 `orm:"column(QueryCounter);type(bigint unsigned)"`
	AbnormalCounter uint64 `orm:"column(AbnormalCounter);type(bigint unsigned)"`
}

// RecordQueryAction record query and action
func (m *MySQL) RecordQueryAction(context sql.QueryContext, action string) error {
	logger.Debugf("action: %s", action)

	// 异步记录
	go func() {
		o := orm.NewOrm()
		var queryAction QueryAction
		queryAction.Query = string(context.Query)
		queryAction.User = string(context.User)
		queryAction.Client = fourByteBigEndianToIP(context.Client)
		queryAction.Db = string(context.Database)
		queryAction.Time = context.Time
		queryAction.Action = action
		id, err := o.Insert(&queryAction)
		if err != nil {
			logger.Warningf("RecordQuery: %s", err.Error())
		} else {
			logger.Debugf("Query saved, ID: %d", id)
		}
	}()
	return nil
}

// RecordAbnormal record abnormal query
func (m *MySQL) RecordAbnormal(context sql.QueryContext) error {
	atomic.AddUint64(&AbnormalCounter, 1)
	go func() {
		o := orm.NewOrm()
		var abnormal Abnormal
		var sx16 = formatPattern(context.Marshal())
		abnormal.Value = sx16
		id, err := o.Insert(&abnormal)
		if err == nil {
			logger.Debugf("Abnormal saved, ID: %d", id)
		} else {
			logger.Warningf("Abnormal save error: %s", err.Error())
		}
	}()
	return nil
}

// CheckPattern check if pattern exist
func (m *MySQL) CheckPattern(pattern []byte) error {

	return errors.New("Not Impletement")
}

// PutPattern put pattern
func (m *MySQL) PutPattern(pattern []byte, query []byte) error {

	return errors.New("Not Impletement")
}

// DeletePattern delete pattern
func (m *MySQL) DeletePattern(pattern []byte) error {
	go func() {
		o := orm.NewOrm()
		if num, err := o.Delete(&Pattern{Key: string(pattern)}); err == nil {
			logger.Debugf("Pattern delete, num: %d", num)
		} else {
			logger.Warningf("Pattern delete error: %s", err.Error())
		}
	}()
	return nil
}

// Purge local databases
func (m *MySQL) Purge() error {
	o := orm.NewOrm()
	_, err := o.Raw("DROP TABLE IF EXISTS pattern, query_action, abnormal, state;").Exec()
	if err != nil {
		return err
	}
	logger.Warningf("All tables dropped")
	return nil
}

// SyncAndClose local databases
func (m *MySQL) SyncAndClose() error {
	// 由 go-sql-driver/mysql 控制
	logger.Debug("MySql synced and closed")
	return nil
}

func formatPattern(pattern []byte) string {
	return fmt.Sprintf("%x", pattern)
}

func unformatPattern(patterString string) []byte {
	var dst []byte
	akey := []byte(patterString)
	dst = make([]byte, hex.DecodedLen(len(akey)))
	hex.Decode(dst, akey)
	return dst
}

// AddPattern add
func (m *MySQL) AddPattern(pattern []byte, context sql.QueryContext) error {
	// pattern := sql.Pattern(context.Query)
	patternString := formatPattern(pattern)

	atomic.AddUint64(&QueryCounter, 1)
	o := orm.NewOrm()
	exist := o.QueryTable("pattern").Filter("key", patternString).Exist()
	if !exist {
		var aPattern Pattern
		aPattern.Key = patternString
		aPattern.Value = string(context.Query)
		id, err := o.Insert(&aPattern)
		if err == nil {
			logger.Debugf("Pattern saved, ID: %d", id)
		} else {
			logger.Warningf("Pattern saved error: %s", err.Error())
		}
	}
	uKey := bytes.Buffer{}
	uKey.Write(pattern)
	uKey.WriteString("_user_")
	uKey.Write(context.User)
	uKeyString := formatPattern(uKey.Bytes())

	exist = o.QueryTable("pattern").Filter("key", uKeyString).Exist()
	if !exist {
		var aPattern Pattern
		aPattern.Key = uKeyString
		aPattern.Value = formatPattern([]byte{0x11})
		id, err := o.Insert(&aPattern)
		if err == nil {
			logger.Debugf("Pattern User saved, ID: %d", id)
		} else {
			logger.Warningf("Pattern User saved error: %s", err.Error())
		}
	}

	cKey := bytes.Buffer{}
	cKey.Write(pattern)
	cKey.WriteString("_client_")
	cKey.Write(context.Client)
	cKeyString := formatPattern(cKey.Bytes())

	exist = o.QueryTable("pattern").Filter("key", cKeyString).Exist()
	if !exist {
		var aPattern Pattern
		aPattern.Key = cKeyString
		aPattern.Value = formatPattern([]byte{0x11})
		id, err := o.Insert(&aPattern)
		if err == nil {
			logger.Debugf("Pattern Source saved, ID: %d", id)
		} else {
			logger.Warningf("Pattern Source saved error: %s", err.Error())
		}
	}

	return nil
}

//CheckQuery check query
func (m *MySQL) CheckQuery(context sql.QueryContext, checkUser bool, checkSource bool) bool {
	atomic.AddUint64(&QueryCounter, 1)
	pattern := sql.Pattern(context.Query)
	patternString := formatPattern(pattern)
	o := orm.NewOrm()
	exist := o.QueryTable("pattern").Filter("key", patternString).Exist()
	if !exist {
		return false
	}
	key := bytes.Buffer{}
	if checkUser {
		key.Write(pattern)
		key.WriteString("_user_")
		key.Write(context.User)
		exist := o.QueryTable("pattern").Filter("key", formatPattern(key.Bytes())).Exist()
		if !exist {
			return false
		}
	}
	if checkSource {
		key.Reset()
		key.Write(pattern)
		key.WriteString("_client_")
		key.Write(context.Client)
		exist := o.QueryTable("pattern").Filter("key", formatPattern(key.Bytes())).Exist()
		if !exist {
			return false
		}
	}
	return true
}

//UpdateState update
func (m *MySQL) UpdateState() error {
	o := orm.NewOrm()
	var state State
	err := o.QueryTable("state").Filter("key", "state").One(&state)
	if err != nil {
		if err == orm.ErrMultiRows {
			// 多条的时候报错
			logger.Warning("Returned Multi Rows Not One")
		}
		if err == orm.ErrNoRows {
			// 没有找到记录
			logger.Warning("Not row found")
			var newState State
			newState.QueryCounter = QueryCounter
			newState.QueryCounter = AbnormalCounter
			newState.Key = "state"
			id, err := o.Insert(&newState)
			if err == nil {
				logger.Warning(id)
				return nil
			}
			return err
		}
		return err
	}
	state.QueryCounter = QueryCounter
	state.AbnormalCounter = AbnormalCounter
	_, err = o.Update(&state)
	if err == nil {
		logger.Debugf("State Updated, QueryCounter:%d AbnormalCounter:%d", QueryCounter, AbnormalCounter)
		return nil
	}
	return err
}

// Abnormals list abnormals
func (m *MySQL) Abnormals() (count int) {
	var abnormals []*Abnormal
	o := orm.NewOrm()
	_, err := o.QueryTable("abnormal").All(&abnormals)
	if err == nil && len(abnormals) > 0 {
		logger.Debug("range abnormal")
		for _, element := range abnormals {
			var c sql.QueryContext
			c.Unmarshal(unformatPattern(element.Value))
			fmt.Printf("[%s] [User: %s] [Database: %s] %s\n",
				c.Time.Format(time.RFC1123),
				c.User,
				c.Database,
				c.Query)
			count++
		}
	} else {
		logger.Debug("no abnormals")
	}
	return
}

// Patterns list Patterns
func (m *MySQL) Patterns() (count int) {
	logger.Debugf("==> Patterns")
	var patterns []*Pattern
	o := orm.NewOrm()
	_, err := o.QueryTable("pattern").All(&patterns)
	if err == nil {
		logger.Debug(patterns)
		for _, element := range patterns {
			elementKey := unformatPattern(element.Key)
			if strings.Index(string(elementKey), "_client_") == -1 && strings.Index(string(elementKey), "_user_") == -1 {
				fmt.Printf(
					`-----Pattern: 0x%s
Sample: %s
`,
					element.Key,
					element.Value,
				)
				count++
			}
		}
	} else {
		logger.Warningf("Pattern error: %s", err.Error())
	}
	return
}

//InitialDB local databases
func (m *MySQL) InitialDB(str string, syncInterval time.Duration, timeout time.Duration) error {
	orm.Debug = false
	//InitLocalDB initail local db
	orm.RegisterDriver("mysql", orm.DRMySQL)

	err := orm.RegisterDataBase("default", "mysql", str, 30)
	if err != nil {
		// logger.Debugf("%s", err.Error())
		return err
	}
	// 注册定义的model
	orm.RegisterModel(new(QueryAction))
	orm.RegisterModel(new(Pattern))
	orm.RegisterModel(new(Abnormal))
	orm.RegisterModel(new(State))

	// 创建table
	// Database alias.
	name := "default"
	// Drop table and re-create.
	force := false
	// Print log.
	verbose := false
	orm.RunSyncdb(name, force, verbose)
	return nil
}
