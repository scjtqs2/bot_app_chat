package bot

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/scjtqs2/bot_adapter/client"
	"github.com/scjtqs2/bot_adapter/coolq"
	"github.com/scjtqs2/bot_adapter/pb/entity"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// chatgpt的配置
var (
	openaiEndpoint = "https://wulfs-den.ink/proxy/openai/v1/"
	apiKey         = "8d32ffe8-7ead-4a2c-a0a4-38fca06d5449"
)

// init 初始化变量
func init() {
	if os.Getenv("OPENAI_ENDPOINT") != "" {
		openaiEndpoint = os.Getenv("OPENAI_ENDPOINT")
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
}

// ChatGptText 处理文字
func ChatGptText(message string, userID int64, groupID int64, botAdapterClient *client.AdapterService) (string, error) {
	newClient := openai.NewClient(
		// azure.WithEndpoint(azureOpenAIEndpoint, azureOpenAIAPIVersion),
		option.WithBaseURL(openaiEndpoint),
		option.WithAPIKey(apiKey), // defaults to os.LookupEnv("OPENAI_API_KEY")
	)
	msgs := coolq.DeCode(message) // 将字符串格式转成 array格式
	aiMessages := make([]openai.ChatCompletionMessageParamUnion, 0)
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
			log.Info("chatgpt image  url=%s img=%s err=%v", msg.Data["file"], f, err)
			aiMessages = append(aiMessages, openai.UserMessageParts(openai.ImagePart(f)))
		case coolq.TEXT:
			aiMessages = append(aiMessages, openai.UserMessage(msg.Data["text"]))
		}
	}
	if len(aiMessages) == 0 {
		return "", errors.New("empty")
	}
	chatCompletion, err := newClient.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
		Messages: openai.F(aiMessages),
		Model:    openai.F(openai.ChatModelGPT4o),
	})
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(chatCompletion.ID, "error") {
		return "", errors.New(chatCompletion.Choices[0].Message.Content)
	}
	return chatCompletion.Choices[0].Message.Content, nil
}
