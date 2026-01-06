// Package handler 提供 HTTP 请求处理器
// 本文件处理会话相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// OpenSessionHandler 打开/创建会话
// POST /session/openSession
// 请求体: request.OpenSessionRequest
// 响应: string (会话ID)
func OpenSessionHandler(c *gin.Context) {
	var req request.OpenSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	sessionId, err := service.Svc.Session.OpenSession(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, sessionId)
}

// GetUserSessionListHandler 获取单聊会话列表
// GET /session/getUserSessionList?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.UserSessionListRespond
func GetUserSessionListHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Session.GetUserSessionList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetGroupSessionListHandler 获取群聊会话列表
// GET /session/getGroupSessionList?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.GroupSessionListRespond
func GetGroupSessionListHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Session.GetGroupSessionList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// DeleteSessionHandler 删除会话
// POST /session/deleteSession
// 请求体: request.DeleteSessionRequest
// 响应: nil
func DeleteSessionHandler(c *gin.Context) {
	var req request.DeleteSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Session.DeleteSession(req.UserId, req.SessionId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// CheckOpenSessionAllowedHandler 检查是否允许打开会话
// 用于检查两个用户之间的关系是否允许建立会话
// GET /session/checkOpenSessionAllowed?sendId=xxx&receiveId=xxx
// 查询参数: request.CreateSessionRequest
// 响应: bool
func CheckOpenSessionAllowedHandler(c *gin.Context) {
	var req request.CreateSessionRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	allowed, err := service.Svc.Session.CheckOpenSessionAllowed(req.SendId, req.ReceiveId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, allowed)
}
