package bot

import (
	"context"
	"errors"
	"github.com/google/generative-ai-go/genai"
	"github.com/scjtqs2/bot_adapter/client"
	"github.com/scjtqs2/bot_adapter/coolq"
	"github.com/scjtqs2/bot_adapter/pb/entity"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/option"
	"os"
	"strings"
	"time"
)

/**
 * @author scjtqs
 * @email scjtqs@qq.com
 */

// gemini的配置
var (
	GeminiEndpoint = "https://generativelanguage.googleapis.com/"
	GeminiApiKey   = ""
	GeminiModel    = "gemini-2.0-flash"
)

// init 初始化变量
func init() {
	if os.Getenv("GEMINI_ENDPOINT") != "" {
		GeminiEndpoint = os.Getenv("GEMINI_ENDPOINT")
	}
	if os.Getenv("GEMINI_API_KEY") != "" {
		GeminiApiKey = os.Getenv("GEMINI_API_KEY")
	}
	if os.Getenv("GEMINI_MODEL") != "" {
		GeminiModel = os.Getenv("GEMINI_MODEL")
	}
}

// GeminiText 处理文字
func GeminiText(message string, userID int64, groupID int64, botAdapterClient *client.AdapterService) (rsp string, err error) {
	if GeminiApiKey == "" {
		return "", errors.New("empyt openai api key")
	}
	// 配置超时时间
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	// Access your API key as an environment variable (see "Set up your API key" above)
	newClient, err := genai.NewClient(ctx, option.WithAPIKey(GeminiApiKey), option.WithEndpoint(GeminiEndpoint))
	if err != nil {
		log.Error(err)
		return "", err
	}
	defer newClient.Close()

	model := newClient.GenerativeModel(GeminiModel)
	cs := model.StartChat()
	msgs := coolq.DeCode(message) // 将字符串格式转成 array格式
	aiMessages := make([]genai.Part, 0)
	prompt := "你是一个智能助手，你只能用中文回答所有问题。"
	cs.History = append(cs.History, &genai.Content{
		Parts: []genai.Part{
			genai.Text(prompt),
		},
		Role: "user",
	})
	oldMsgLen := 0
	// if groupID != 0 {
	oldMsgs := Msglog.GetMsgs(groupID, userID)
	if oldMsgs != nil {
		oldMsgLen = len(oldMsgs)
		for _, s := range oldMsgs {
			if s.IsSystem {
				cs.History = append(cs.History, &genai.Content{
					Parts: []genai.Part{
						genai.Text(s.Msg),
					},
					Role: "model",
				})
			} else {
				cs.History = append(cs.History, &genai.Content{
					Parts: []genai.Part{
						genai.Text(s.Msg),
					},
					Role: "user",
				})
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
			var imgData []byte
			if strings.HasPrefix(f, "http") {
				r := Request{URL: f, Limit: maxImageSize}
				imgData, err = r.Bytes()
				if err != nil {
					log.Errorf("r.Bytes() faild err=%v", err)
				}
			} else if strings.HasPrefix(f, "file") {
				img, err := botAdapterClient.GetImage(context.TODO(), &entity.GetImageReq{File: f})
				if err != nil {
					return "", err
				}
				r := Request{URL: img.File, Limit: maxImageSize}
				imgData, err = r.Bytes()
				if err != nil {
					log.Errorf("r.Bytes() faild err=%v", err)
				}
			}
			aiMessages = append(aiMessages, genai.ImageData("jpeg", imgData))
		case coolq.TEXT:
			aiMessages = append(aiMessages, genai.Text(msg.Data["text"]))
			Msglog.AddMsg(groupID, userID, msg.Data["text"], false)
		}
	}
	if len(aiMessages) == oldMsgLen {
		return "", errors.New("empty")
	}

	resp, err := cs.SendMessage(ctx, aiMessages...)
	if err != nil {
		return "", err
	}
	rspText := ""
	if resp.Candidates != nil {
		for _, v := range resp.Candidates {
			for _, k := range v.Content.Parts {
				// fmt.Println(k.(genai.Text))
				rspText += string(k.(genai.Text))
			}
		}
	}
	return rspText, err
}
