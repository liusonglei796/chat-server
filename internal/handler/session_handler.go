// Package handler 提供 HTTP 请求处理器
// 本文件处理会话相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request/contact"
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
// 从JWT上下文获取当前用户ID
// 响应: map[string]interface{} (list, total, page, page_size)
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
	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	data, total, err := h.sessionSvc.GetUserSessionList(userId.(string), page, pageSize)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, map[string]interface{}{
		"list":      data,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetGroupSessionList 获取群聊会话列表
// GET /session/getGroupSessionList?page=1&page_size=20
// 从JWT上下文获取当前用户ID
// 响应: map[string]interface{} (list, total, page, page_size)
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
	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	data, total, err := h.sessionSvc.GetGroupSessionList(userId.(string), page, pageSize)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, map[string]interface{}{
		"list":      data,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
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

	var req contact.BatchDeleteRequest
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
