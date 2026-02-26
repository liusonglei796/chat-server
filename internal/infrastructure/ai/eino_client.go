package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"kama_chat_server/internal/config"
)

// ChatClient AI 对话客户端抽象
type ChatClient interface {
	Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// EinoClient 基于 Eino 的模型客户端
type EinoClient struct {
	model *openai.ChatModel
}

// NewEinoClient 创建 Eino 客户端
func NewEinoClient(cfg config.ModelScopeConfig) (*EinoClient, error) {
	if strings.TrimSpace(cfg.ApiKey) == "" {
		return nil, fmt.Errorf("modelscope api key is empty")
	}
	if strings.TrimSpace(cfg.BaseUrl) == "" {
		return nil, fmt.Errorf("modelscope base url is empty")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("modelscope model is empty")
	}

	temperature := float32(0.3)
	chatModel, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		APIKey:      cfg.ApiKey,
		BaseURL:     cfg.BaseUrl,
		Model:       cfg.Model,
		Temperature: &temperature,
		Timeout:     12 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	return &EinoClient{model: chatModel}, nil
}

// Generate 调用模型生成文本
func (c *EinoClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(userPrompt),
	}

	outMsg, err := c.model.Generate(ctx, messages)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(outMsg.Content), nil
}
