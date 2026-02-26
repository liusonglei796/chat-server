// Package translation 提供翻译服务
package translation

import (
	"context"
	"fmt"
	"strings"

	einopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
)

// TranslationService 翻译服务
type TranslationService struct {
	chatModel *einopenai.ChatModel
}

// NewTranslationService 创建翻译服务实例
func NewTranslationService(apiKey, baseURL, model string) (*TranslationService, error) {
	chatModel, err := einopenai.NewChatModel(context.Background(), &einopenai.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	})
	if err != nil {
		return nil, err
	}

	return &TranslationService{
		chatModel: chatModel,
	}, nil
}

// Translate 将文本翻译成目标语言
func (s *TranslationService) Translate(ctx context.Context, text, targetLang string) (string, error) {
	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是一个专业的翻译助手。",
		},
		{
			Role: schema.User,
			Content: fmt.Sprintf("请将下面的消息翻译成%s，只返回翻译结果，不要有任何解释：\n\n%s",
				targetLang,
				text,
			),
		},
	}

	resp, err := s.chatModel.Generate(ctx, messages)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("empty translation response")
	}

	translatedText := strings.TrimSpace(resp.Content)
	if translatedText == "" {
		return "", fmt.Errorf("empty translation result")
	}

	return translatedText, nil
}
