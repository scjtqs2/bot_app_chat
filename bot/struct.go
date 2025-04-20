package bot

import (
	"encoding/json"
	"fmt"
	"github.com/syndtr/goleveldb/leveldb"
	"sync"
)

var Msglog *MsgLog

// MSG 消息Map
type MSG map[string]interface{}

type MsgLog struct {
	db    *leveldb.DB
	lock  sync.Mutex
	lenth int
}

const (
	MsgTypeText  = "" // 默认为空，兼容之前的
	MsgTypeImage = "image"
)

type MsgObj struct {
	IsSystem bool   `json:"is_system"`
	Msg      string `json:"msg"`
	msgType  string `json:"msg_type"` // 消息类型
}

func init() {
	db, err := leveldb.OpenFile("/data/msgdb", nil)
	if err != nil {
		panic(err)
	}
	Msglog = &MsgLog{db: db, lenth: 30}
}

func (m *MsgLog) AddMsg(groupid, userid int64, text string, isSystem bool, msgType string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	key := m.MakeKey(groupid, userid)
	msgs, _ := m.db.Get([]byte(key), nil)
	if msgs == nil {
		msgs = []byte("{}")
	}
	var msgsArr []MsgObj
	json.Unmarshal(msgs, &msgsArr)
	msgsArr = append(msgsArr, MsgObj{IsSystem: isSystem, Msg: text, msgType: msgType})
	l := len(msgsArr)
	if l > m.lenth {
		msgsArr = msgsArr[l-m.lenth:]
	}
	buf, _ := json.Marshal(msgsArr)
	m.db.Put([]byte(key), buf, nil)
}

func (m *MsgLog) MakeKey(groupid, userid int64) string {
	return fmt.Sprintf("@chatgpt/group/%d/user/%d", groupid, userid)
}

func (m *MsgLog) GetMsgs(groupid, userid int64) []MsgObj {
	m.lock.Lock()
	defer m.lock.Unlock()
	key := m.MakeKey(groupid, userid)
	msgs, _ := m.db.Get([]byte(key), nil)
	if msgs == nil {
		return nil
	}
	var msgsArr []MsgObj
	json.Unmarshal(msgs, &msgsArr)
	return msgsArr
}
