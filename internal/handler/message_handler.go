// Package handler 提供 HTTP 请求处理器
// 本文件处理消息和文件上传相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request"
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

// GetMessageList 获取两个用户之间的聊天记录
// GET /message/getMessageList?userOneId=xxx&userTwoId=xxx
// 查询参数: request.GetMessageListRequest
// 响应: []respond.GetMessageListRespond
// 安全: 从JWT上下文获取当前用户ID，校验调用者是聊天双方之一
func (h *MessageHandler) GetMessageList(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.GetMessageListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	// 权限校验: 调用者必须是聊天双方之一
	currentUserId := userId.(string)
	if currentUserId != req.UserOneId && currentUserId != req.UserTwoId {
		HandleError(c, errorx.New(errorx.CodeForbidden, "无权查看此聊天记录"))
		return
	}

	data, err := h.messageSvc.GetMessageList(req.UserOneId, req.UserTwoId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetGroupMessageList 获取群聊消息记录
// GET /message/getGroupMessageList?groupId=xxx
// 查询参数: request.GetGroupMessageListRequest
// 响应: []respond.GetGroupMessageListRespond
// 安全: 从JWT上下文获取当前用户ID，Service层校验群成员身份
func (h *MessageHandler) GetGroupMessageList(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.GetGroupMessageListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.messageSvc.GetGroupMessageList(userId.(string), req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
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
