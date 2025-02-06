package bot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/scjtqs2/bot_adapter/client"
	"github.com/scjtqs2/bot_adapter/coolq"
	"github.com/scjtqs2/bot_adapter/pb/entity"
	log "github.com/sirupsen/logrus"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/syndtr/goleveldb/leveldb"
)

// chatgpt的配置
var (
	// OpenaiEndpoint = "https://wulfs-den.ink/proxy/openai/v1/"
	OpenaiEndpoint = "https://api.openai.com/v1/"
	OpenaiApiKey   = ""
	OpenaiModel    = openai.ChatModelGPT4oMini
	Msglog         *MsgLog
)

type MsgLog struct {
	db    *leveldb.DB
	lock  sync.Mutex
	lenth int
}

type MsgObj struct {
	IsSystem bool   `json:"is_system"`
	Msg      string `json:"msg"`
}

// init 初始化变量
func init() {
	if os.Getenv("OPENAI_ENDPOINT") != "" {
		OpenaiEndpoint = os.Getenv("OPENAI_ENDPOINT")
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		OpenaiApiKey = os.Getenv("OPENAI_API_KEY")
	}
	if os.Getenv("OPENAI_MODEL") != "" {
		OpenaiModel = os.Getenv("OPENAI_MODEL")
	}
	db, err := leveldb.OpenFile("/data/msgdb", nil)
	if err != nil {
		panic(err)
	}
	Msglog = &MsgLog{db: db, lenth: 30}
}

// ChatGptText 处理文字
func ChatGptText(message string, userID int64, groupID int64, botAdapterClient *client.AdapterService) (rsp string, err error) {
	newClient := openai.NewClient(
		// azure.WithEndpoint(azureOpenAIEndpoint, azureOpenAIAPIVersion),
		option.WithBaseURL(OpenaiEndpoint),
		option.WithAPIKey(OpenaiApiKey), // defaults to os.LookupEnv("OPENAI_API_KEY")
	)
	msgs := coolq.DeCode(message) // 将字符串格式转成 array格式
	aiMessages := make([]openai.ChatCompletionMessageParamUnion, 0)
	oldMsgLen := 0
	// if groupID != 0 {
	oldMsgs := Msglog.GetMsgs(groupID, userID)
	if oldMsgs != nil {
		oldMsgLen = len(oldMsgs)
		for _, s := range oldMsgs {
			if s.IsSystem {
				aiMessages = append(aiMessages, openai.SystemMessage(s.Msg))
			} else {
				aiMessages = append(aiMessages, openai.UserMessage(s.Msg))
			}
		}
	}
	// }
	defer func() {
		if err == nil {
			Msglog.AddMsg(groupID, userID, rsp, true)
		}
	}()
	for _, msg := range msgs {
		var err error
		switch msg.Type {
		case coolq.IMAGE:
			f := msg.Data["file"]
			if strings.HasPrefix(f, "http") {
				var b []byte
				r := Request{URL: f, Limit: maxImageSize}
				b, err = r.Bytes()
				if err != nil {
					log.Errorf("r.Bytes() faild err=%v", err)
				}
				f = fmt.Sprintf("data:image/jpeg;base64,%s", base64.StdEncoding.EncodeToString(b))
			} else if strings.HasPrefix(f, "file") {
				img, err := botAdapterClient.GetImage(context.TODO(), &entity.GetImageReq{File: f})
				if err != nil {
					return "", err
				}
				r := Request{URL: img.File, Limit: maxImageSize}
				b, err := r.Bytes()
				if err != nil {
					log.Errorf("r.Bytes() faild err=%v", err)
				}
				f = fmt.Sprintf("data:image/jpeg;base64,%s", base64.StdEncoding.EncodeToString(b))
			}
			// log.Info("chatgpt image  url=%s img=%s err=%v", msg.Data["file"], f, err)
			aiMessages = append(aiMessages, openai.UserMessageParts(openai.ImagePart(f)))
		case coolq.TEXT:
			aiMessages = append(aiMessages, openai.UserMessage(msg.Data["text"]))
			Msglog.AddMsg(groupID, userID, msg.Data["text"], false)
		}
	}
	if len(aiMessages) == oldMsgLen {
		return "", errors.New("empty")
	}
	// 配置超时时间
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	chatCompletion, err := newClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: openai.F(aiMessages),
		Model:    openai.F(OpenaiModel),
		// MaxTokens: openai.Int(1000),
	},
	// This sets the per-retry timeout
	// option.WithRequestTimeout(5*time.Minute),
	)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(chatCompletion.ID, "error") {
		return "", errors.New(chatCompletion.JSON.RawJSON())
	}
	return chatCompletion.Choices[0].Message.Content, nil
}

func (m *MsgLog) AddMsg(groupid, userid int64, text string, isSystem bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
	key := m.MakeKey(groupid, userid)
	msgs, _ := m.db.Get([]byte(key), nil)
	if msgs == nil {
		msgs = []byte("{}")
	}
	var msgsArr []MsgObj
	json.Unmarshal(msgs, &msgsArr)
	msgsArr = append(msgsArr, MsgObj{IsSystem: isSystem, Msg: text})
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
