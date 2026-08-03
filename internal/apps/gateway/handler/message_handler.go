// Package handler 提供 HTTP 请求处理器
// 本文件处理消息相关的 API 请求
package handler

import (
	"kama_chat_server/internal/common/dto/request/message"
	"kama_chat_server/internal/common/grpc_client"
	messagepb "kama_chat_server/api/gen/message"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// MessageHandler 消息请求处理器
type MessageHandler struct {
}

// NewMessageHandler 创建消息处理器实例
func NewMessageHandler() *MessageHandler {
	return &MessageHandler{}
}

// GetMessageList 获取两人的聊天记录（游标分页）
func (h *MessageHandler) GetMessageList(c *gin.Context) {
	ctx := c.Request.Context()

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

	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	rsp, err := grpc_client.MessageClient.GetMessageListCursor(ctx, &messagepb.GetMessageListCursorRequest{
		RequesterId: userId.(string),
		PartnerId:   req.TargetId,
		Cursor:      req.Cursor,
		PageSize:    int32(pageSize),
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, gin.H{
		"list":        rsp.List,
		"next_cursor": rsp.NextCursor,
		"has_more":    rsp.HasMore,
	})
}

// GetGroupMessageList 获取群聊的聊天记录（游标分页）
func (h *MessageHandler) GetGroupMessageList(c *gin.Context) {
	ctx := c.Request.Context()

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
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	rsp, err := grpc_client.MessageClient.GetGroupMessageListCursor(ctx, &messagepb.GetGroupMessageListCursorRequest{
		UserId:   userId.(string),
		GroupId:  req.TargetId,
		Cursor:   req.Cursor,
		PageSize: int32(pageSize),
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, gin.H{
		"list":        rsp.List,
		"next_cursor": rsp.NextCursor,
		"has_more":    rsp.HasMore,
	})
}

// RecallMessage 撤回消息
func (h *MessageHandler) RecallMessage(c *gin.Context) {
	ctx := c.Request.Context()

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

	if _, err := grpc_client.MessageClient.RecallMessage(ctx, &messagepb.RecallMessageRequest{
		UserId:      userId.(string),
		MessageUuid: req.MessageUuid,
	}); err != nil {
		HandleError(c, err)
		return
	}

	HandleSuccess(c, nil)
}

// UploadAvatar 上传头像
func (h *MessageHandler) UploadAvatar(c *gin.Context) {
	HandleError(c, errorx.New(errorx.CodeInvalidParam, "请在网关独立实现或使用 OSS 直传"))
}

// UploadFile 上传文件
func (h *MessageHandler) UploadFile(c *gin.Context) {
	HandleError(c, errorx.New(errorx.CodeInvalidParam, "请在网关独立实现或使用 OSS 直传"))
}
