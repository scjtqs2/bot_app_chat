package bot

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scjtqs2/bot_adapter/client"
	"github.com/scjtqs2/bot_adapter/coolq"
	"github.com/scjtqs2/bot_adapter/pb/entity"
	log "github.com/sirupsen/logrus"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// chatgpt的配置
var (
	// OpenaiEndpoint = "https://wulfs-den.ink/proxy/openai/v1/"
	OpenaiEndpoint = "https://api.openai.com/v1/"
	OpenaiApiKey   = ""
	OpenaiModel    = openai.ChatModelGPT4oMini
)

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
}

// ChatGptText 处理文字
func ChatGptText(message string, userID int64, groupID int64, botAdapterClient *client.AdapterService) (rsp string, err error) {
	if OpenaiApiKey == "" {
		return "", errors.New("empyt openai api key")
	}
	newClient := openai.NewClient(
		// azure.WithEndpoint(azureOpenAIEndpoint, azureOpenAIAPIVersion),
		option.WithBaseURL(OpenaiEndpoint),
		option.WithAPIKey(OpenaiApiKey), // defaults to os.LookupEnv("OPENAI_API_KEY")
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
			switch s.msgType {
			case MsgTypeText:
				if s.IsSystem {
					aiMessages = append(aiMessages, openai.SystemMessage(s.Msg))
				} else {
					aiMessages = append(aiMessages, openai.UserMessage(s.Msg))
				}
			case MsgTypeImage:
				if s.IsSystem {
					// 暂时不支持
				} else {
					aiMessages = append(aiMessages, openai.UserMessageParts(openai.ChatCompletionContentPartImageParam{
						Type: openai.F(openai.ChatCompletionContentPartImageTypeImageURL),
						ImageURL: openai.F(openai.ChatCompletionContentPartImageImageURLParam{
							URL:    openai.F(s.Msg),
							Detail: openai.F(openai.ChatCompletionContentPartImageImageURLDetailHigh),
						}),
					}))
				}
			}

		}
	}
	// }
	defer func() {
		if err == nil {
			Msglog.AddMsg(groupID, userID, rsp, true, MsgTypeText, "")
		}
	}()
	for _, msg := range msgs {
		var err error
		switch msg.Type {
		case coolq.IMAGE:
			f := msg.Data["file"]
			contentType := ""
			mimeType := "image/jpeg"
			if strings.HasPrefix(f, "http") {
				var b []byte
				r := Request{URL: f, Limit: maxImageSize}
				b, contentType, err = r.Bytes()
				if err != nil {
					log.Errorf("r.Bytes() faild err=%v", err)
				}
				if strings.Contains(contentType, "png") {
					mimeType = "image/png"
				}
				f = fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(b))
			} else if strings.HasPrefix(f, "file") {
				img, err := botAdapterClient.GetImage(context.TODO(), &entity.GetImageReq{File: f})
				if err != nil {
					return "", err
				}
				var b []byte
				r := Request{URL: img.File, Limit: maxImageSize}
				b, contentType, err = r.Bytes()
				if err != nil {
					log.Errorf("r.Bytes() faild err=%v", err)
				}
				if strings.Contains(contentType, "png") {
					mimeType = "image/png"
				}
				f = fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(b))
			}
			// log.Info("chatgpt image  url=%s img=%s err=%v", msg.Data["file"], f, err)
			// aiMessages = append(aiMessages, openai.UserMessageParts(openai.ImagePart(f)))
			aiMessages = append(aiMessages, openai.UserMessageParts(openai.ChatCompletionContentPartImageParam{
				Type: openai.F(openai.ChatCompletionContentPartImageTypeImageURL),
				ImageURL: openai.F(openai.ChatCompletionContentPartImageImageURLParam{
					URL:    openai.F(f),
					Detail: openai.F(openai.ChatCompletionContentPartImageImageURLDetailHigh),
				}),
			}))
			Msglog.AddMsg(groupID, userID, f, false, MsgTypeImage, mimeType)
		case coolq.TEXT:
			aiMessages = append(aiMessages, openai.UserMessage(msg.Data["text"]))
			Msglog.AddMsg(groupID, userID, msg.Data["text"], false, MsgTypeText, "")
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
