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
	OpenaiEndpoint        = "https://api.openai.com/v1/"
	OpenaiAPIKey          = ""
	OpenaiModel           = openai.ChatModelGPT4oMini
	OpenaiReasoningEffort = ""    // 推理努力级别：low|medium|high（适用于o1等推理模型）
	OpenaiImageUseBase64  = false // 图片是否使用base64方式而非URL方式
)

// init 初始化变量
func init() {
	if os.Getenv("OPENAI_ENDPOINT") != "" {
		OpenaiEndpoint = os.Getenv("OPENAI_ENDPOINT")
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		OpenaiAPIKey = os.Getenv("OPENAI_API_KEY")
	}
	if os.Getenv("OPENAI_MODEL") != "" {
		OpenaiModel = os.Getenv("OPENAI_MODEL")
	}
	if os.Getenv("OPENAI_REASONING_EFFORT") != "" {
		OpenaiReasoningEffort = os.Getenv("OPENAI_REASONING_EFFORT")
	}
	if os.Getenv("OPENAI_IMAGE_USE_BASE64") != "" {
		OpenaiImageUseBase64 = os.Getenv("OPENAI_IMAGE_USE_BASE64") == "true" || os.Getenv("OPENAI_IMAGE_USE_BASE64") == "1"
	}
}

// ChatGptText 处理文字
func ChatGptText(message string, userID int64, groupID int64, botAdapterClient *client.AdapterService) (rsp string, err error) {
	if OpenaiAPIKey == "" {
		return "", errors.New("empyt openai api key")
	}
	newClient := openai.NewClient(
		// azure.WithEndpoint(azureOpenAIEndpoint, azureOpenAIAPIVersion),
		option.WithBaseURL(OpenaiEndpoint),
		option.WithAPIKey(OpenaiAPIKey), // defaults to os.LookupEnv("OPENAI_API_KEY")
	)
	msgs := coolq.DeCode(message) // 将字符串格式转成 array格式
	aiMessages := make([]openai.ChatCompletionMessageParamUnion, 0)
	prompt := "你是一个智能助手，你只能用中文回答所有问题。不要使用markdown语法，我不能解析它，请使用纯文本"
	aiMessages = append(aiMessages, openai.SystemMessage(prompt))
	oldMsgLen := 0
	// if groupID != 0 {
	oldMsgs := Msglog.GetMsgs(groupID, userID)
	if oldMsgs != nil {
		oldMsgLen = len(oldMsgs)
		for _, s := range oldMsgs {
			switch s.MsgType {
			case MsgTypeText:
				if s.IsSystem {
					// 系统消息只能在开头，历史消息中的系统消息作为assistant消息处理
					aiMessages = append(aiMessages, openai.AssistantMessage(s.Msg))
				} else {
					aiMessages = append(aiMessages, openai.UserMessage(s.Msg))
				}
			case MsgTypeImage:
				if !s.IsSystem {
					// 历史消息中的图片尝试两种方式都支持
					parts := []openai.ChatCompletionContentPartUnionParam{
						{
							OfImageURL: &openai.ChatCompletionContentPartImageParam{
								ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
									URL:    s.Msg,
									Detail: "high",
								},
							},
						},
					}
					aiMessages = append(aiMessages, openai.UserMessage(parts))
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
		log.Debugf("msg: %v", msg)
		var err error
		switch msg.Type {
		case coolq.IMAGE:
			f := msg.Data["file"]
			if !strings.HasPrefix(f, "http") && !strings.HasPrefix(f, "file") && !strings.HasPrefix(f, "base64://") {
				if u, ok := msg.Data["url"]; ok && u != "" {
					f = u
				}
			}
			contentType := ""
			mimeType := "image/jpeg"
			var imageURL string
			switch {
			case strings.HasPrefix(f, "http"):
				if OpenaiImageUseBase64 {
					var b []byte
					r := Request{URL: f, Limit: maxImageSize}
					b, contentType, err = r.Bytes()
					if err != nil {
						log.Errorf("http r.Bytes() failed err=%v", err)
						continue // 修复点：下载失败必须跳过，绝不能把空数据发给 API
					}
					if strings.Contains(contentType, "png") {
						mimeType = "image/png"
					}
					imageURL = fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(b))
				} else {
					imageURL = f
				}
			case strings.HasPrefix(f, "file"):
				img, err := botAdapterClient.GetImage(context.TODO(), &entity.GetImageReq{File: f})
				if err != nil {
					log.Errorf("GetImage failed err=%v", err)
					continue // 修复点：获取图片失败必须跳过
				}
				var b []byte
				r := Request{URL: img.File, Limit: maxImageSize}
				b, contentType, err = r.Bytes()
				if err != nil {
					log.Errorf("file r.Bytes() failed err=%v", err)
					continue // 修复点：下载失败必须跳过
				}
				if strings.Contains(contentType, "png") {
					mimeType = "image/png"
				}
				imageURL = fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(b))
			case strings.HasPrefix(f, "base64://"):
				b64Str := strings.TrimPrefix(f, "base64://")
				imageURL = fmt.Sprintf("data:%s;base64,%s", mimeType, b64Str)
			default:
				// 修复点 2：防止不知名的文件格式绕过校验发给 API
				if !strings.HasPrefix(f, "http") && !strings.HasPrefix(f, "data:") {
					log.Warnf("未知的图片前缀格式，抛弃该图片避免 API 报错: %s", f)
					continue // 直接跳过这个图片，不继续往下组装 parts
				}
			}
			parts := []openai.ChatCompletionContentPartUnionParam{
				{
					OfImageURL: &openai.ChatCompletionContentPartImageParam{
						ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
							URL:    imageURL,
							Detail: "high",
						},
					},
				},
			}
			log.Debugf("image f=%s imageURL=%s", f, imageURL)
			aiMessages = append(aiMessages, openai.UserMessage(parts))
			Msglog.AddMsg(groupID, userID, imageURL, false, MsgTypeImage, mimeType)
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

	// 构建请求参数
	params := openai.ChatCompletionNewParams{
		Messages: aiMessages,
		Model:    OpenaiModel,
		// MaxTokens: openai.Int(1000),
	}

	// 设置推理努力级别（适用于 o-series 模型）
	switch OpenaiReasoningEffort {
	case "low":
		params.ReasoningEffort = openai.ReasoningEffortLow
	case "medium":
		params.ReasoningEffort = openai.ReasoningEffortMedium
	case "high":
		params.ReasoningEffort = openai.ReasoningEffortHigh
	}

	chatCompletion, err := newClient.Chat.Completions.New(ctx, params)

	if err != nil {
		return "", err
	}
	if len(chatCompletion.Choices) == 0 {
		return "", errors.New("no choices returned")
	}
	return chatCompletion.Choices[0].Message.Content, nil
}
