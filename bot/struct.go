package bot

import (
	"encoding/json"
	"fmt"
	"github.com/syndtr/goleveldb/leveldb"
	"sync"
)

// Msglog 全局消息日志实例
var Msglog *MsgLog

// MSG 消息Map
type MSG map[string]interface{}

// MsgLog 消息日志结构
type MsgLog struct {
	db    *leveldb.DB
	lock  sync.Mutex
	lenth int
}

// 消息类型常量
const (
	MsgTypeText  = "" // 默认为空，兼容之前的
	MsgTypeImage = "image"
)

// MsgObj 消息对象
type MsgObj struct {
	IsSystem bool   `json:"is_system"`
	Msg      string `json:"msg"`
	MsgType  string `json:"msg_type"`  // 消息类型
	MimeType string `json:"mime_type"` // 图片类型
}

func init() {
	db, err := leveldb.OpenFile("/data/msgdb", nil)
	if err != nil {
		panic(err)
	}
	Msglog = &MsgLog{db: db, lenth: 30}
}

// AddMsg 添加消息
func (m *MsgLog) AddMsg(groupid, userid int64, text string, isSystem bool, msgType string, mimeType string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	key := m.MakeKey(groupid, userid)
	msgs, _ := m.db.Get([]byte(key), nil)
	if msgs == nil {
		msgs = []byte("{}")
	}
	var msgsArr []MsgObj
	_ = json.Unmarshal(msgs, &msgsArr)
	msgsArr = append(msgsArr, MsgObj{IsSystem: isSystem, Msg: text, MsgType: msgType, MimeType: mimeType})
	l := len(msgsArr)
	if l > m.lenth {
		msgsArr = msgsArr[l-m.lenth:]
	}
	buf, _ := json.Marshal(msgsArr)
	_ = m.db.Put([]byte(key), buf, nil)
}

// MakeKey 生成消息存储的键
func (m *MsgLog) MakeKey(groupid, userid int64) string {
	return fmt.Sprintf("@chatgpt/group/%d/user/%d", groupid, userid)
}

// GetMsgs 获取历史消息
func (m *MsgLog) GetMsgs(groupid, userid int64) []MsgObj {
	m.lock.Lock()
	defer m.lock.Unlock()
	key := m.MakeKey(groupid, userid)
	msgs, _ := m.db.Get([]byte(key), nil)
	if msgs == nil {
		return nil
	}
	var msgsArr []MsgObj
	_ = json.Unmarshal(msgs, &msgsArr)
	return msgsArr
}
