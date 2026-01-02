package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// OpenSession 打开会话
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

// GetUserSessionList 获取用户会话列表
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

// GetGroupSessionList 获取群聊会话列表
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

// DeleteSession 删除会话
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

// CheckOpenSessionAllowed 检查是否可以打开会话
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
