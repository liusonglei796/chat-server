// Package handler 提供 HTTP 请求处理器
// 本文件处理消息和文件上传相关的 API 请求
package handler

import (
	"kama_chat_server/internal/service"

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
func (h *MessageHandler) GetMessageList(c *gin.Context) {
	var req struct {
		UserOneId string `form:"userOneId" binding:"required"`
		UserTwoId string `form:"userTwoId" binding:"required"`
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
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
func (h *MessageHandler) GetGroupMessageList(c *gin.Context) {
	var req struct {
		GroupId string `form:"groupId" binding:"required"`
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.messageSvc.GetGroupMessageList(req.GroupId)
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
