// Package handler 提供 HTTP 请求处理器
// 本文件处理会话相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request/friendship"
	"kama_chat_server/internal/dto/request/session"
	"kama_chat_server/internal/service"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// SessionHandler 会话请求处理器
// 通过构造函数注入 SessionService，遵循依赖倒置原则
type SessionHandler struct {
	sessionSvc service.SessionService
}

// NewSessionHandler 创建会话处理器实例
// sessionSvc: 会话服务接口
func NewSessionHandler(sessionSvc service.SessionService) *SessionHandler {
	return &SessionHandler{sessionSvc: sessionSvc}
}

// OpenSession 打开/创建会话
// POST /session/openSession
// 请求体: session.OpenSessionRequest (只需 receive_id)
// 响应: string (会话ID)
// 安全: 从JWT上下文获取当前用户ID作为sendId，防止IDOR攻击
func (h *SessionHandler) OpenSession(c *gin.Context) {
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

	// 直接使用 JWT 中的 userId 作为 sendId，无需比较
	sessionId, err := h.sessionSvc.OpenSession(userId.(string), req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, sessionId)
}

// GetUserSessionList 获取单聊会话列表
// GET /session/getUserSessionList?page=1&page_size=20
// GET /session/getUserSessionList?cursor=1234567890&page_size=20 (推荐使用游标分页)
// 从JWT上下文获取当前用户ID
// 响应: map[string]interface{} (list, total, page, page_size) 或 (list, cursor, has_more, page_size)
func (h *SessionHandler) GetUserSessionList(c *gin.Context) {
	// 从JWT中间件获取当前用户ID
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

	// 设置默认分页参数
	if req.PageSize <= 0 {
		req.PageSize = 20
	}

	// 优先使用游标分页（推荐）
	if req.Cursor != "" {
		h.getUserSessionListWithCursor(c, userId.(string), req)
		return
	}

	// 兼容传统分页（已不推荐，但保持向后兼容）
	if req.Page <= 0 {
		req.Page = 1
	}
	h.getUserSessionListWithPage(c, userId.(string), req)
}

// getUserSessionListWithPage 使用传统分页（已不推荐，但保持向后兼容）
func (h *SessionHandler) getUserSessionListWithPage(c *gin.Context, userId string, req session.GetSessionListRequest) {
	data, total, err := h.sessionSvc.GetUserSessionList(userId, req.Page, req.PageSize)
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

// getUserSessionListWithCursor 使用游标分页（推荐）
func (h *SessionHandler) getUserSessionListWithCursor(c *gin.Context, userId string, req session.GetSessionListRequest) {
	data, nextCursor, hasMore, err := h.sessionSvc.GetUserSessionListCursor(userId, req.Cursor, req.PageSize)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, map[string]interface{}{
		"list":      data,
		"cursor":    nextCursor,
		"has_more":  hasMore,
		"page_size": req.PageSize,
	})
}

// GetGroupSessionList 获取群聊会话列表
// GET /session/getGroupSessionList?page=1&page_size=20
// GET /session/getGroupSessionList?cursor=1234567890&page_size=20 (推荐使用游标分页)
// 从JWT上下文获取当前用户ID
// 响应: map[string]interface{} (list, total, page, page_size) 或 (list, cursor, has_more, page_size)
func (h *SessionHandler) GetGroupSessionList(c *gin.Context) {
	// 从JWT中间件获取当前用户ID
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

	// 设置默认分页参数
	if req.PageSize <= 0 {
		req.PageSize = 20
	}

	// 优先使用游标分页（推荐）
	if req.Cursor != "" {
		h.getGroupSessionListWithCursor(c, userId.(string), req)
		return
	}

	// 兼容传统分页（已不推荐，但保持向后兼容）
	if req.Page <= 0 {
		req.Page = 1
	}
	h.getGroupSessionListWithPage(c, userId.(string), req)
}

// getGroupSessionListWithPage 使用传统分页（已不推荐，但保持向后兼容）
func (h *SessionHandler) getGroupSessionListWithPage(c *gin.Context, userId string, req session.GetSessionListRequest) {
	data, total, err := h.sessionSvc.GetGroupSessionList(userId, req.Page, req.PageSize)
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

// getGroupSessionListWithCursor 使用游标分页（推荐）
func (h *SessionHandler) getGroupSessionListWithCursor(c *gin.Context, userId string, req session.GetSessionListRequest) {
	data, nextCursor, hasMore, err := h.sessionSvc.GetGroupSessionListCursor(userId, req.Cursor, req.PageSize)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, map[string]interface{}{
		"list":      data,
		"cursor":    nextCursor,
		"has_more":  hasMore,
		"page_size": req.PageSize,
	})
}

// DeleteSession 删除会话
// POST /session/deleteSession
// 请求体: request.DeleteSessionRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，防止IDOR攻击
func (h *SessionHandler) DeleteSession(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req friendship.BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	// 从 UuidList 中取第一个（单个操作）
	if len(req.UuidList) == 0 {
		HandleError(c, errorx.New(errorx.CodeInvalidParam, "uuid_list 不能为空"))
		return
	}
	if err := h.sessionSvc.DeleteSession(userId.(string), req.UuidList[0]); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// CheckOpenSessionAllowed 检查是否允许打开会话
// 用于检查两个用户之间的关系是否允许建立会话
// GET /session/checkOpenSessionAllowed?receiveId=xxx
// 查询参数: session.CheckSessionAllowedRequest (只需 receive_id)
// 响应: bool
// 安全: 从JWT上下文获取当前用户ID作为sendId，防止IDOR攻击
func (h *SessionHandler) CheckOpenSessionAllowed(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req session.CheckSessionAllowedRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	// 安全: 使用 JWT 中的用户 ID 作为 sendId
	allowed, err := h.sessionSvc.CheckOpenSessionAllowed(userId.(string), req.ReceiveId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, allowed)
}

// PinSession 置顶/取消置顶会话
// PUT /sessions/pin
// 请求体: session.PinSessionRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，Service层校验会话归属
func (h *SessionHandler) PinSession(c *gin.Context) {
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

	if err := h.sessionSvc.PinSession(userId.(string), req.SessionId, req.IsPinned); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
