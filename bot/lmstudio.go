package bot

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/scjtqs2/bot_adapter/client"
	"github.com/scjtqs2/bot_adapter/coolq"
	"github.com/scjtqs2/bot_adapter/pb/entity"
	log "github.com/sirupsen/logrus"
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
	LmStudioEndpoint = ""
	LmStudioApiKey   = ""
	LmStudioModel    = ""
)

// init 初始化变量
func init() {
	if os.Getenv("LMSTUDIO_ENDPOINT") != "" {
		LmStudioEndpoint = os.Getenv("LMSTUDIO_ENDPOINT")
	}
	if os.Getenv("LMSTUDIO_API_KEY") != "" {
		LmStudioApiKey = os.Getenv("LMSTUDIO_API_KEY")
	}
	if os.Getenv("LMSTUDIO_MODEL") != "" {
		LmStudioModel = os.Getenv("LMSTUDIO_MODEL")
	}
}

// LmStudioText 处理文字
func LmStudioText(message string, userID int64, groupID int64, botAdapterClient *client.AdapterService) (rsp string, err error) {
	if LmStudioEndpoint == "" || LmStudioModel == "" {
		return "", errors.New("empyt lmstudio api")
	}
	newClient := openai.NewClient(
		// azure.WithEndpoint(azureOpenAIEndpoint, azureOpenAIAPIVersion),
		option.WithBaseURL(LmStudioEndpoint),
		option.WithAPIKey(LmStudioApiKey), // defaults to os.LookupEnv("OPENAI_API_KEY")
	)
	msgs := coolq.DeCode(message) // 将字符串格式转成 array格式
	aiMessages := make([]openai.ChatCompletionMessageParamUnion, 0)
	prompt := "你是一个智能助手，你只能用中文回答所有问题。"
	aiMessages = append(aiMessages, openai.SystemMessage(prompt))
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	chatCompletion, err := newClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: openai.F(aiMessages),
		Model:    openai.F(LmStudioModel),
		// MaxTokens: openai.Int(1000),
	},
		// option.WithRequestTimeout(5*time.Minute),
	)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(chatCompletion.ID, "error") || len(chatCompletion.Choices) == 0 {
		return "", errors.New(chatCompletion.JSON.RawJSON())
	}
	return chatCompletion.Choices[0].Message.Content, nil
}
