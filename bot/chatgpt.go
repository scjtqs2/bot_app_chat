package bot

import (
	"context"
	"errors"
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
func ChatGptText(message string, userID int64, groupID int64) (string, error) {
	client := openai.NewClient(
		// azure.WithEndpoint(azureOpenAIEndpoint, azureOpenAIAPIVersion),
		option.WithBaseURL(openaiEndpoint),
		option.WithAPIKey(apiKey), // defaults to os.LookupEnv("OPENAI_API_KEY")
	)
	chatCompletion, err := client.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(message),
		}),
		Model: openai.F(openai.ChatModelGPT4o),
	})
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(chatCompletion.ID, "error") {
		return "", errors.New(chatCompletion.Choices[0].Message.Content)
	}
	return chatCompletion.Choices[0].Message.Content, nil
}
