package bot

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/scjtqs2/bot_adapter/client"
	"github.com/scjtqs2/bot_adapter/coolq"
	"github.com/scjtqs2/bot_adapter/pb/entity"
	log "github.com/sirupsen/logrus"
	"google.golang.org/genai"
)

/**
 * @author scjtqs
 * @email scjtqs@qq.com
 */

// gemini的配置
var (
	GeminiEndpoint           = "https://generativelanguage.googleapis.com"
	GeminiAPIKey             = ""
	GeminiModel              = "gemini-1.5-flash"
	GeminiProxy              = ""   // 代理地址，例如 http://127.0.0.1:7890
	GeminiInsecureSkipVerify = true // 是否跳过TLS证书验证
)

// init 初始化变量
func init() {
	if os.Getenv("GEMINI_ENDPOINT") != "" {
		GeminiEndpoint = os.Getenv("GEMINI_ENDPOINT")
	}
	if os.Getenv("GEMINI_API_KEY") != "" {
		GeminiAPIKey = os.Getenv("GEMINI_API_KEY")
	}
	if os.Getenv("GEMINI_MODEL") != "" {
		GeminiModel = os.Getenv("GEMINI_MODEL")
	}
	if os.Getenv("GEMINI_PROXY") != "" {
		GeminiProxy = os.Getenv("GEMINI_PROXY")
	}
	if os.Getenv("GEMINI_INSECURE_SKIP_VERIFY") != "" {
		GeminiInsecureSkipVerify = os.Getenv("GEMINI_INSECURE_SKIP_VERIFY") == "true" || os.Getenv("GEMINI_INSECURE_SKIP_VERIFY") == "1"
	}
}

// GeminiText 处理文字
func GeminiText(message string, userID int64, groupID int64, botAdapterClient *client.AdapterService) (rsp string, err error) {
	if GeminiAPIKey == "" {
		return "", errors.New("empty gemini api key")
	}
	// 配置超时时间
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	// 构建客户端配置
	clientConfig := &genai.ClientConfig{
		APIKey:  GeminiAPIKey,
		Backend: genai.BackendGeminiAPI,
	}

	// 如果配置了代理或跳过TLS验证，创建自定义HTTP客户端
	if GeminiProxy != "" || GeminiInsecureSkipVerify {
		httpClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: GeminiInsecureSkipVerify,
				},
			},
		}

		// 设置代理
		if GeminiProxy != "" {
			proxyURL, err := url.Parse(GeminiProxy)
			if err == nil {
				httpClient.Transport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)
			} else {
				log.Errorf("Invalid proxy URL: %v", err)
			}
		}

		clientConfig.HTTPClient = httpClient
	}

	// 创建新的 genai 客户端
	newClient, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		log.Error(err)
		return "", err
	}

	// 构建历史消息
	var history []*genai.Content

	// 添加系统提示
	prompt := "你是一个智能助手，你只能用中文回答所有问题。"
	history = append(history, genai.NewContentFromText(prompt, genai.RoleUser))
	history = append(history, genai.NewContentFromText("好的", genai.RoleModel))

	// 添加历史消息
	oldMsgs := Msglog.GetMsgs(groupID, userID)
	for _, s := range oldMsgs {
		switch s.MsgType {
		case MsgTypeText:
			if s.IsSystem {
				history = append(history, genai.NewContentFromText(s.Msg, genai.RoleModel))
			} else {
				history = append(history, genai.NewContentFromText(s.Msg, genai.RoleUser))
			}
		case MsgTypeImage:
			parts := []*genai.Part{
				genai.NewPartFromBytes([]byte(s.Msg), s.MimeType),
			}
			if s.IsSystem {
				history = append(history, genai.NewContentFromParts(parts, genai.RoleModel))
			} else {
				history = append(history, genai.NewContentFromParts(parts, genai.RoleUser))
			}
		}
	}

	defer func() {
		if err == nil {
			Msglog.AddMsg(groupID, userID, rsp, true, MsgTypeText, "")
		}
	}()

	msgs := coolq.DeCode(message) // 将字符串格式转成 array格式
	if len(msgs) == 0 {
		return "", errors.New("empty")
	}

	var parts []*genai.Part
	for _, msg := range msgs {
		switch msg.Type {
		case coolq.IMAGE:
			f := msg.Data["file"]
			if !strings.HasPrefix(f, "http") && !strings.HasPrefix(f, "file") && !strings.HasPrefix(f, "base64://") {
				if u, ok := msg.Data["url"]; ok && u != "" {
					f = u
				}
			}
			var imgData []byte
			contentType := ""
			mimeType := "image/jpeg"
			if strings.HasPrefix(f, "http") {
				r := Request{URL: f, Limit: maxImageSize}
				imgData, contentType, err = r.Bytes()
				if err != nil {
					log.Errorf("r.Bytes() failed err=%v", err)
				}
				if strings.Contains(contentType, "png") {
					mimeType = "image/png"
				}
			} else if strings.HasPrefix(f, "file") {
				img, err := botAdapterClient.GetImage(context.TODO(), &entity.GetImageReq{File: f})
				if err != nil {
					return "", err
				}
				r := Request{URL: img.File, Limit: maxImageSize}
				imgData, contentType, err = r.Bytes()
				if err != nil {
					log.Errorf("r.Bytes() failed err=%v", err)
				}
				if strings.Contains(contentType, "png") {
					mimeType = "image/png"
				}
			} else {
				if !strings.HasPrefix(f, "http") && !strings.HasPrefix(f, "data:") && !strings.HasPrefix(f, "base64://") {
					log.Warnf("未知的图片前缀格式，抛弃该图片避免 API 报错: %s", f)
					continue // 直接跳过这个图片，不继续往下组装 parts
				}
			}
			parts = append(parts, genai.NewPartFromBytes(imgData, mimeType))
			Msglog.AddMsg(groupID, userID, string(imgData), false, MsgTypeImage, mimeType)
		case coolq.TEXT:
			parts = append(parts, genai.NewPartFromText(msg.Data["text"]))
			Msglog.AddMsg(groupID, userID, msg.Data["text"], false, MsgTypeText, "")
		}
	}

	// 创建聊天会话 - 注意参数顺序: model, config, history
	chat, err := newClient.Chats.Create(ctx, GeminiModel, nil, history)
	if err != nil {
		return "", err
	}

	// 发送消息
	resp, err := chat.Send(ctx, parts...)
	if err != nil {
		return "", err
	}

	// 提取响应文本
	var rspText string
	if resp.Candidates != nil {
		for _, candidate := range resp.Candidates {
			if candidate.Content != nil {
				for _, part := range candidate.Content.Parts {
					if part.Text != "" {
						rspText += part.Text
					}
				}
			}
		}
	}
	if rspText == "" {
		return "", fmt.Errorf("empty response from gemini")
	}
	return rspText, nil
}
