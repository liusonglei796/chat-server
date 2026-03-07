package ai

import (
	"context"
	"strings"
	"time"

	"kama_chat_server/internal/config"
	"kama_chat_server/pkg/errorx"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
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
		return nil, errorx.New(errorx.CodeInvalidParam, "modelscope api key is empty")
	}
	if strings.TrimSpace(cfg.BaseUrl) == "" {
		return nil, errorx.New(errorx.CodeInvalidParam, "modelscope base url is empty")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errorx.New(errorx.CodeInvalidParam, "modelscope model is empty")
	}

	temperature := float32(0.3)
	// Eino OpenAI 兼容模型初始化入口：创建可复用的 ChatModel 实例。
	// 这里配置的是 ModelScope 的 OpenAI-Compatible 网关参数（APIKey/BaseURL/Model）。
	chatModel, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		APIKey:      cfg.ApiKey,
		BaseURL:     cfg.BaseUrl,
		Model:       cfg.Model,
		Temperature: &temperature,
		Timeout:     12 * time.Second,
	})
	if err != nil {
		return nil, errorx.Wrap(err, errorx.CodeServerBusy, "AI模型初始化失败")
	}

	return &EinoClient{model: chatModel}, nil
}

// Generate 调用模型生成文本
func (c *EinoClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Eino 消息协议：按角色组织上下文。
	// schema.SystemMessage: 系统指令（定义模型行为与输出格式）
	// schema.UserMessage: 用户输入（本次任务内容）
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(userPrompt),
	}

	// Eino 核心推理调用：向大模型发送 messages，返回 assistant 消息。
	// 返回的 outMsg.Content 是模型文本结果，后续由业务层做 JSON 解析。
	outMsg, err := c.model.Generate(ctx, messages)
	if err != nil {
		return "", errorx.Wrap(err, errorx.CodeServerBusy, "AI模型调用失败")
	}

	return strings.TrimSpace(outMsg.Content), nil
}
