// Package handler 提供 HTTP 请求处理器
// 本文件处理消息和文件上传相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request/message"
	"kama_chat_server/internal/service"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// MessageHandler 消息请求处理器
// 通过构造函数注入 MessageService，遵循依赖倒置原则
type MessageHandler struct {
	messageSvc service.MessageService
}

// NewMessageHandler 创建消息处理器实例
// messageSvc: 消息服务接口
func NewMessageHandler(messageSvc service.MessageService) *MessageHandler {
	return &MessageHandler{messageSvc: messageSvc}
}

// GetMessageList 获取聊天记录（私聊 + 群聊统一入口）
// GET /messages?target_id=xxx&page=1&page_size=20
// target_id 以 "U" 开头表示私聊，以 "G" 开头表示群聊
// 查询参数: message.GetMessageListRequest
// 响应: map[string]interface{} (list, total, page, page_size)
// 安全: 从JWT上下文获取当前用户ID，防止查看他人聊天记录
func (h *MessageHandler) GetMessageList(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req message.GetMessageListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	// 设置默认分页参数
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}

	var data interface{}
	var total int64
	var err error

	// 根据 TargetId 前缀自动派发
	if len(req.TargetId) > 0 && req.TargetId[0] == 'G' {
		data, total, err = h.messageSvc.GetGroupMessageList(userId.(string), req.TargetId, req.Page, req.PageSize)
	} else {
		data, total, err = h.messageSvc.GetMessageList(userId.(string), req.TargetId, req.Page, req.PageSize)
	}

	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, map[string]interface{}{
		"list":      data,
		"total":     total,
		"page":      req.Page,
		"page_size": req.PageSize,
	})
}

// UploadAvatar 上传用户头像
// POST /message/uploadAvatar
// 请求体: multipart/form-data
// 响应: string (新头像文件名)
// 限制: 仅支持 image/jpeg, image/png, image/gif
func (h *MessageHandler) UploadAvatar(c *gin.Context) {
	path, err := h.messageSvc.UploadAvatar(c)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, path)
}

// UploadFile 上传聊天文件
// POST /message/uploadFile
// 请求体: multipart/form-data
// 响应: []string (上传成功的文件名列表)
func (h *MessageHandler) UploadFile(c *gin.Context) {
	paths, err := h.messageSvc.UploadFile(c)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, paths)
}

// RecallMessage 撤回消息
// POST /messages/recall
// 请求体: message.RecallMessageRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，Service层校验发送者身份
func (h *MessageHandler) RecallMessage(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req message.RecallMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if err := h.messageSvc.RecallMessage(userId.(string), req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
