// Package translation 提供翻译服务
package translation

import (
	"context"

	openai "github.com/sashabaranov/go-openai"
)

// TranslationService 翻译服务
type TranslationService struct {
	client *openai.Client
	model  string
}

// NewTranslationService 创建翻译服务实例
func NewTranslationService(apiKey, baseURL, model string) *TranslationService {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL
	client := openai.NewClientWithConfig(config)

	return &TranslationService{
		client: client,
		model:  model,
	}
}

// Translate 将文本翻译成目标语言
func (s *TranslationService) Translate(ctx context.Context, text, targetLang string) (string, error) {
	prompt := "你是一个专业的翻译助手。请将下面的消息翻译成" + targetLang + "，只返回翻译结果，不要有任何解释：\n\n" + text

	resp, err := s.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: s.model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "你是一个专业的翻译助手。",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		},
	)
	if err != nil {
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}
