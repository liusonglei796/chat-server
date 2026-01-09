// Package handler 提供 HTTP 请求处理器
// 本文件处理会话相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

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
// 请求体: request.OpenSessionRequest
// 响应: string (会话ID)
func (h *SessionHandler) OpenSession(c *gin.Context) {
	var req request.OpenSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	sessionId, err := h.sessionSvc.OpenSession(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, sessionId)
}

// GetUserSessionList 获取单聊会话列表
// GET /session/getUserSessionList?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.UserSessionListRespond
func (h *SessionHandler) GetUserSessionList(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.sessionSvc.GetUserSessionList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetGroupSessionList 获取群聊会话列表
// GET /session/getGroupSessionList?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.GroupSessionListRespond
func (h *SessionHandler) GetGroupSessionList(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.sessionSvc.GetGroupSessionList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// DeleteSession 删除会话
// POST /session/deleteSession
// 请求体: request.DeleteSessionRequest
// 响应: nil
func (h *SessionHandler) DeleteSession(c *gin.Context) {
	var req request.DeleteSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.sessionSvc.DeleteSession(req.UserId, req.SessionId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// CheckOpenSessionAllowed 检查是否允许打开会话
// 用于检查两个用户之间的关系是否允许建立会话
// GET /session/checkOpenSessionAllowed?sendId=xxx&receiveId=xxx
// 查询参数: request.CreateSessionRequest
// 响应: bool
func (h *SessionHandler) CheckOpenSessionAllowed(c *gin.Context) {
	var req request.CreateSessionRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	allowed, err := h.sessionSvc.CheckOpenSessionAllowed(req.SendId, req.ReceiveId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, allowed)
}
