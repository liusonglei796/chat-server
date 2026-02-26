// Package handler 提供 HTTP 请求处理器
// 本文件处理 AI 相关的 API 请求
package handler

import (
	aireq "kama_chat_server/internal/dto/request/ai"
	"kama_chat_server/internal/service"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// AIHandler AI 请求处理器
type AIHandler struct {
	aiSvc service.AIService
}

// NewAIHandler 创建 AI 处理器实例
func NewAIHandler(aiSvc service.AIService) *AIHandler {
	return &AIHandler{aiSvc: aiSvc}
}

// ReplySuggestions 智能回复建议
// POST /ai/reply-suggestions
func (h *AIHandler) ReplySuggestions(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req aireq.ReplySuggestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	rsp, err := h.aiSvc.ReplySuggestions(userId.(string), req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, rsp)
}

// GroupSummary 群聊总结
// POST /ai/group-summary
func (h *AIHandler) GroupSummary(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req aireq.GroupSummaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	rsp, err := h.aiSvc.GroupSummary(userId.(string), req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, rsp)
}

// Translate 多语言翻译
// POST /ai/translate
func (h *AIHandler) Translate(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req aireq.TranslateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	rsp, err := h.aiSvc.Translate(userId.(string), req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, rsp)
}
