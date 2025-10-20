package main

// 对接 https://github.com/scjtqs2/docker-asterisk-freepbx/tree/main/sms_send 的短信发送接口。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scjtqs2/bot_adapter/event"
	"github.com/scjtqs2/bot_adapter/pb/entity"
	log "github.com/sirupsen/logrus"
)

// --- SMS 状态管理 ---

// SMSState 存储用户发送短信的会话状态
type SMSState struct {
	// ================== 1. 修改结构体 ==================
	Device      string `json:"device"`
	PhoneNumber string `json:"phone_number"`
	Message     string `json:"message"`
	Step        string `json:"step"` // "awaiting_message" 或 "awaiting_confirmation"
}

const (
	StateAwaitingMessage      = "awaiting_message"
	StateAwaitingConfirmation = "awaiting_confirmation"
)

var (
	// userSmsState 存储每个用户的短信会话状态
	userSmsState = make(map[int64]*SMSState)
	// smsStateLock 保护 userSmsState 的并发访问
	smsStateLock = &sync.Mutex{}

	// allowedSmsUserIDs 声明，将在 init() 中被填充
	allowedSmsUserIDs map[int64]bool

	// SMS API 配置
	smsApiUrl    string
	smsApiSecret string
	smsApiClient *http.Client

	// isSmsFeatureEnabled 标记短信功能是否全局启用
	isSmsFeatureEnabled bool
)

// SMSSendRequest 定义了调用 API 所需的结构体
type SMSSendRequest struct {
	Secret    string `json:"secret"`
	Device    string `json:"device"`
	Recipient string `json:"recipient"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// APIResponse 定义了 SMS API 的响应结构
type APIResponse struct {
	Status  string `json:"status"` // "success" 或 "error"
	Message string `json:"message"`
}

func init() {
	// 检查短信功能是否全局启用
	smsEnabledEnv := os.Getenv("SMS_FEATURE_ENABLED")
	isSmsFeatureEnabled = (strings.ToLower(smsEnabledEnv) == "true" || smsEnabledEnv == "1")
	log.Infof("SMS 短信功能已启用: %v", isSmsFeatureEnabled)

	// 如果功能未启用，可以提前退出 init 中的 SMS 相关设置
	if !isSmsFeatureEnabled {
		log.Info("SMS 短信功能未启用，跳过 API 和用户配置。")
		return
	}

	// 从环境变量加载 SMS API 配置
	smsApiUrl = os.Getenv("SMS_API_URL")
	if smsApiUrl == "" {
		// 使用你提供的 URL 作为后备
		smsApiUrl = "http://192.168.50.124:1285/api/v1/sms/send"
		log.Warnf("SMS_API_URL 未设置, 使用默认值: %s", smsApiUrl)
	}
	// 这个 Secret 必须与你的 http_handler 服务中设置的 FORWARD_SECRET 环境变量一致
	smsApiSecret = os.Getenv("SMS_API_SECRET")
	if smsApiSecret == "" {
		log.Warn("SMS_API_SECRET 未设置。短信发送可能会因认证失败。")
	}

	smsApiClient = &http.Client{
		Timeout: 30 * time.Second, // 30秒超时
	}
	log.Infof("SMS API Handler 初始化, URL: %s", smsApiUrl)

	// 初始化 allowedSmsUserIDs map
	allowedSmsUserIDs = make(map[int64]bool)

	// 从环境变量加载授权用户ID
	usersEnv := os.Getenv("SMS_ALLOWED_USERS")
	if usersEnv == "" {
		log.Warn("SMS_ALLOWED_USERS 环境变量未设置, 短信功能将对所有非管理员用户禁用。")
		// 注意：这里没有 return，允许 init() 的其余部分继续执行
	} else {
		// 按逗号分割ID
		idStrings := strings.Split(usersEnv, ",")
		loadedCount := 0
		for _, idStr := range idStrings {
			trimmedID := strings.TrimSpace(idStr)
			if trimmedID == "" {
				continue
			}

			// 解析 ID
			id, err := strconv.ParseInt(trimmedID, 10, 64)
			if err != nil {
				log.Warnf("无法解析 SMS_ALLOWED_USERS 中的 ID: '%s'，已跳过", idStr)
				continue
			}

			// 添加到 map 中
			allowedSmsUserIDs[id] = true
			loadedCount++
		}
		log.Infof("成功从 SMS_ALLOWED_USERS 加载 %d 个授权用户 ID", loadedCount)
	}
}

// --- SMS 核心处理逻辑 ---

// handlePrivateSmsConversation 检查并处理私聊中的短信发送流程
// 返回 'true' 表示消息已被短信流程处理，'false' 表示应由 AI 继续处理
func handlePrivateSmsConversation(req event.MessagePrivate) bool {
	// 0. 检查功能是否全局启用
	if !isSmsFeatureEnabled {
		return false // 功能未启用，交由 AI 处理
	}

	smsStateLock.Lock()
	defer smsStateLock.Unlock()

	userID := req.UserID
	message := strings.TrimSpace(req.RawMessage)

	// 1. 检查 /cancel 命令
	if message == "/cancel" {
		if _, ok := userSmsState[userID]; ok {
			delete(userSmsState, userID)
			sendReply(userID, "操作已取消。")
			return true // 消息已处理
		}
	}

	// 2. 检查用户是否已处于某个会话状态
	state, inState := userSmsState[userID]

	if !inState {
		// 3. 用户不在会话中，检查是否为 /send 启动命令
		if strings.HasPrefix(message, "/send ") {
			// 3.1 检查权限
			if _, allowed := allowedSmsUserIDs[userID]; !allowed {
				sendReply(userID, "您没有权限使用这个 bot。")
				return true // 消息已处理（已拒绝）
			}

			// ================== 2. 修改 /send 解析逻辑 ==================
			// 3.2 解析命令 (格式: /send <device> <phone_number>)
			parts := strings.Split(message, " ")
			if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
				sendReply(userID, "请按照以下格式发送命令：/send <device> <phone_number>")
				return true // 消息已处理
			}

			// 3.3 进入下一步：等待消息内容
			device := parts[1]
			phoneNumber := parts[2]
			userSmsState[userID] = &SMSState{
				Device:      device, // 存储 device
				PhoneNumber: phoneNumber,
				Step:        StateAwaitingMessage,
			}
			sendReply(userID, "请输入您要发送的信息：")
			return true // 消息已处理
		}

		// 4. 非短信命令，且不在会话中，交由 AI 处理
		return false
	}

	// 5. 用户处于会话中，根据步骤处理
	switch state.Step {
	case StateAwaitingMessage:
		// ================== 3. 修改确认信息 ==================
		// 5.1 接收到消息内容，进入确认步骤
		state.Message = message
		state.Step = StateAwaitingConfirmation
		replyText := fmt.Sprintf("请确认信息：\n设备：%s\n手机号：%s\n信息：%s\n\n(回复 'yes' 确认发送，回复 /cancel 取消)", state.Device, state.PhoneNumber, state.Message)
		sendReply(userID, replyText)
		return true // 消息已处理

	case StateAwaitingConfirmation:
		// 5.2 接收到确认信息
		confirmation := strings.ToLower(message)
		if confirmation == "yes" {
			// 确认发送
			log.Infof("用户 %d 确认发送短信到 %s (设备: %s)", userID, state.PhoneNumber, state.Device)

			// ================== 4. 修改 API 调用 ==================
			// 调用 HTTP API 发送短信
			result, err := sendSmsViaAPI(state.Device, state.PhoneNumber, state.Message) // 传入 state.Device
			var replyText string
			if err != nil {
				replyText = fmt.Sprintf("发送失败：\n%s", err.Error())
			} else {
				replyText = fmt.Sprintf("发送结果：\n%s", result)
			}
			sendReply(userID, replyText)
		} else {
			// 取消发送
			sendReply(userID, "操作已取消。")
		}
		// 无论成功、失败还是取消，都清除会话状态
		delete(userSmsState, userID)
		return true // 消息已处理
	}

	return false // 理论上不会到达这里
}

// --- 辅助函数 ---

// sendSmsViaAPI 执行 HTTP POST 请求到 SMS 服务
func sendSmsViaAPI(device, recipient, message string) (string, error) {
	if smsApiUrl == "" {
		return "", fmt.Errorf("SMS_API_URL 未配置")
	}

	// ================== 5. 移除硬编码 ==================
	// (移除了 if device == "" { device = "quectel0" } )
	// device 现在完全由调用方 (即用户的 /send 命令) 决定

	// 1. 构建请求体
	reqPayload := SMSSendRequest{
		Secret:    smsApiSecret, // 从环境变量中读取
		Device:    device,       // 使用传入的 device
		Recipient: recipient,
		Message:   message,
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		log.Errorf("SMS API: 序列化 JSON 失败: %v", err)
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}

	// 2. 创建 HTTP 请求
	req, err := http.NewRequest("POST", smsApiUrl, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Errorf("SMS API: 创建 HTTP 请求失败: %v", err)
		return "", fmt.Errorf("创建 HTTP 请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 3. 发送请求
	log.Infof("SMS API: 正在发送请求到 %s (Device: %s)", smsApiUrl, device)
	resp, err := smsApiClient.Do(req)
	if err != nil {
		log.Errorf("SMS API: 请求失败: %v", err)
		return "", fmt.Errorf("请求 SMS API 失败: %v", err)
	}
	defer resp.Body.Close()

	// 4. 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("SMS API: 读取响应体失败: %v", err)
		return "", fmt.Errorf("读取 API 响应失败: %v", err)
	}

	log.Infof("SMS API: 收到响应 (Status: %d): %s", resp.StatusCode, string(body))

	// 5. 解析 JSON 响应
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		// 如果 JSON 解析失败，返回原始响应体
		return "", fmt.Errorf("API 响应解析失败 (Status: %d): %s", resp.StatusCode, string(body))
	}

	// 6. 根据响应状态返回
	if resp.StatusCode >= 400 || (apiResp.Status != "" && apiResp.Status != "success") {
		return "", fmt.Errorf("API 返回错误: %s", apiResp.Message)
	}

	// 返回 http_handler.go 响应中的 "message" 字段
	return apiResp.Message, nil
}

// sendReply 是一个辅助函数，用于向私聊用户发送回复
func sendReply(userID int64, text string) {
	if botAdapterClient == nil {
		log.Errorf("botAdapterClient 未初始化，无法发送私聊回复")
		return
	}
	_, err := botAdapterClient.SendPrivateMsg(context.Background(), &entity.SendPrivateMsgReq{
		UserId:  userID,
		Message: []byte(text),
	})
	if err != nil {
		log.Errorf("向用户 %d 发送私聊回复失败: %v", userID, err)
	}
}
