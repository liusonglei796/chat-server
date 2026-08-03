// Package handler 提供 HTTP 请求处理器
// 本文件处理会话相关的 API 请求
package handler

import (
	"kama_chat_server/internal/common/dto/request/session"
	"kama_chat_server/internal/common/grpc_client"
	messagepb "kama_chat_server/api/gen/message"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// SessionHandler 会话请求处理器
type SessionHandler struct {
}

// NewSessionHandler 创建会话处理器实例
func NewSessionHandler() *SessionHandler {
	return &SessionHandler{}
}

// OpenSession 打开会话
func (h *SessionHandler) OpenSession(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req session.OpenSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	// 权限校验
	allowedRsp, err := grpc_client.MessageClient.CheckOpenSessionAllowed(ctx, &messagepb.CheckOpenSessionAllowedRequest{
		SendId:    userId.(string),
		ReceiveId: req.ReceiveId,
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	if !allowedRsp.Allowed {
		HandleError(c, errorx.New(errorx.CodeForbidden, "不允许发起会话"))
		return
	}

	rsp, err := grpc_client.MessageClient.OpenSession(ctx, &messagepb.OpenSessionRequest{
		SendId:    userId.(string),
		ReceiveId: req.ReceiveId,
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, gin.H{
		"session_id": rsp.SessionId,
	})
}

// GetUserSessionList 获取用户会话列表（游标分页）
func (h *SessionHandler) GetUserSessionList(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req session.GetSessionListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	rsp, err := grpc_client.MessageClient.GetUserSessionListCursor(ctx, &messagepb.GetUserSessionListCursorRequest{
		OwnerId:  userId.(string),
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

// GetGroupSessionList 获取群组会话列表（游标分页）
func (h *SessionHandler) GetGroupSessionList(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req session.GetSessionListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	rsp, err := grpc_client.MessageClient.GetGroupSessionListCursor(ctx, &messagepb.GetGroupSessionListCursorRequest{
		OwnerId:  userId.(string),
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

// DeleteSession 删除会话
func (h *SessionHandler) DeleteSession(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req struct {
		SessionId string `json:"session_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if _, err := grpc_client.MessageClient.DeleteSession(ctx, &messagepb.DeleteSessionRequest{
		OwnerId:   userId.(string),
		SessionId: req.SessionId,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// CheckOpenSessionAllowed 检查是否允许发起会话
func (h *SessionHandler) CheckOpenSessionAllowed(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}
	targetId := c.Query("target_id")
	if targetId == "" {
		HandleError(c, errorx.New(errorx.CodeInvalidParam, "target_id不能为空"))
		return
	}
	rsp, err := grpc_client.MessageClient.CheckOpenSessionAllowed(c.Request.Context(), &messagepb.CheckOpenSessionAllowedRequest{
		SendId:    userId.(string),
		ReceiveId: targetId,
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, gin.H{"allowed": rsp.Allowed})
}

// PinSession 置顶会话
func (h *SessionHandler) PinSession(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req session.PinSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if _, err := grpc_client.MessageClient.PinSession(ctx, &messagepb.PinSessionRequest{
		UserId:    userId.(string),
		SessionId: req.SessionId,
		IsPinned:  req.IsPinned,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
