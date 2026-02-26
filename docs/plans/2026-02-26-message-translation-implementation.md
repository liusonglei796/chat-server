# Message Translation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为 KamaChat IM 系统增加消息翻译功能，用户可以手动触发将聊天消息翻译成英语。

**Architecture:** 使用 ModelScope 平台的 Kimi K2.5 模型作为翻译引擎，通过 OpenAI 兼容 API 调用。后端新增 Translation Service 处理翻译逻辑，复用现有的 handler → service → dao 分层架构。

**Tech Stack:** Go, OpenAI SDK (go-openai), ModelScope API, Gin

---

## Task 1: 配置管理 - 新增 ModelScope 配置

**Files:**
- Modify: `configs/config.toml`
- Modify: `internal/config/config.go` (如需要)

**Step 1: 添加配置项**

在 `configs/config.toml` 末尾添加:

```toml
[modelScopeConfig]
apiKey = "efe04ce3-afcf-43e4-b67e-f57937a9aa5b"
baseUrl = "https://api-inference.modelscope.cn/v1"
model = "moonshotai/Kimi-K2.5"
```

**Step 2: 验证配置格式**

检查现有 config.toml 结构，确保添加位置正确。

---

## Task 2: 创建翻译请求 DTO

**Files:**
- Create: `internal/dto/request/message/translate_request.go`

**Step 1: 创建请求 DTO**

```go
package message

// TranslateRequest 翻译请求
type TranslateRequest struct {
	MessageID  string `json:"messageId" binding:"required"`  // 消息ID
	TargetLang string `json:"targetLang"`                     // 目标语言，默认 en
}
```

**Step 2: 验证文件创建**

确认文件路径正确，package 名称为 `message`。

---

## Task 3: 创建翻译响应 DTO

**Files:**
- Create: `internal/dto/respond/message/translate_respond.go`

**Step 1: 创建响应 DTO**

```go
package message

// TranslateRespond 翻译响应
type TranslateRespond struct {
	OriginalText   string `json:"originalText"`    // 原文
	TranslatedText string `json:"translatedText"` // 译文
}
```

---

## Task 4: 创建 Translation Service

**Files:**
- Create: `internal/service/translation/service.go`

**Step 1: 创建翻译服务**

```go
package translation

import (
	"context"
	openai "github.com/sashabaranov/go-openai"
)

type TranslationService struct {
	client  *openai.Client
	model   string
}

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
			Messages: []openai.ChatMessage{
				openai.SystemMessage("你是一个专业的翻译助手。"),
				openai.UserMessage(prompt),
			},
		},
	)
	if err != nil {
		return "", err
	}
	
	return resp.Choices[0].Message.Content, nil
}
```

**Step 2: 添加 go-openai 依赖**

```bash
go get github.com/sashabaranov/go-openai
```

---

## Task 5: 在 Service Provider 中注册 Translation Service

**Files:**
- Modify: `internal/service/provider.go`

**Step 1: 添加 TranslationService 到 Services 结构体**

在 Services 结构体中添加:
```go
Translation *translation.TranslationService
```

**Step 2: 在 NewServices 函数中初始化**

在 NewServices 函数中添加:
```go
Translation: translation.NewTranslationService(
    cfg.ModelScope.APIKey,
    cfg.ModelScope.BaseURL,
    cfg.ModelScope.Model,
),
```

---

## Task 6: 在 Handler 中添加翻译接口

**Files:**
- Modify: `internal/handler/message_handler.go`

**Step 1: 在 MessageHandler 中添加翻译服务依赖**

```go
type MessageHandler struct {
	messageSvc    service.MessageService
	translationSvc *translation.TranslationService
}
```

**Step 2: 修改 NewMessageHandler 构造函数**

```go
func NewMessageHandler(messageSvc service.MessageService, translationSvc *translation.TranslationService) *MessageHandler {
	return &MessageHandler{
		messageSvc:    messageSvc,
		translationSvc: translationSvc,
	}
}
```

**Step 3: 添加 Translate 方法**

```go
// Translate 翻译消息
func (h *MessageHandler) Translate(c *gin.Context) {
	userId, _ := c.Get("userId")
	
	var req message.TranslateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, -2, "参数错误")
		return
	}
	
	// 根据 messageId 查询消息内容
	msg, err := h.messageSvc.GetMessageByUuid(req.MessageID)
	if err != nil {
		response.Error(c, -2, "消息不存在")
		return
	}
	
	// 检查权限（消息发送者或接收者才能翻译）
	if msg.SendId != userId.(string) && msg.ReceiveId != userId.(string) {
		response.Error(c, -2, "无权限翻译此消息")
		return
	}
	
	// 调用翻译服务
	targetLang := req.TargetLang
	if targetLang == "" {
		targetLang = "英语"
	}
	
	translatedText, err := h.translationSvc.Translate(c.Request.Context(), msg.Content, targetLang)
	if err != nil {
		zap.L().Error("translate failed", zap.Error(err))
		response.Error(c, -1, "翻译失败")
		return
	}
	
	response.Success(c, message.TranslateRespond{
		OriginalText:   msg.Content,
		TranslatedText: translatedText,
	})
}
```

---

## Task 7: 在 MessageService 中添加获取单条消息的方法

**Files:**
- Modify: `internal/service/message/service.go`
- Modify: `internal/service/interfaces.go`

**Step 1: 在 MessageService 接口中添加方法**

```go
// GetMessageByUuid 根据 UUID 获取消息
GetMessageByUuid(messageId string) (*model.Message, error)
```

**Step 2: 实现方法**

```go
// GetMessageByUuid 根据 UUID 获取消息
func (m *messageService) GetMessageByUuid(messageId string) (*model.Message, error) {
	return m.repos.Message.FindByUuid(messageId)
}
```

---

## Task 8: 注册翻译路由

**Files:**
- Modify: `internal/router/message_routes.go`

**Step 1: 添加翻译路由**

```go
messageGroup.POST("/translate", rt.handlers.Message.Translate)
```

---

## Task 9: 在 Handler Provider 中传递翻译服务

**Files:**
- Modify: `internal/handler/provider.go`

**Step 1: 修改 NewHandlers 函数**

```go
func NewHandlers(svc *service.Services, broker chat.MessageBroker) *Handlers {
	return &Handlers{
		Message: NewMessageHandler(svc.Message, svc.Translation),
		// ...
	}
}
```

---

## Task 10: 测试翻译功能

**Step 1: 启动服务**

```bash
go run cmd/kama_chat_server/main.go
```

**Step 2: 测试翻译接口**

```bash
curl -X POST http://localhost:8080/api/message/translate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"messageId": "消息UUID", "targetLang": "英语"}'
```

**Step 3: 验证响应**

预期响应:
```json
{
    "code": 0,
    "msg": "success",
    "data": {
        "originalText": "你好",
        "translatedText": "Hello"
    }
}
```

---

## Task 11: 提交代码

**Step 1: 提交所有更改**

```bash
git add .
git commit -m "feat: add message translation feature using Kimi K2.5"
```
