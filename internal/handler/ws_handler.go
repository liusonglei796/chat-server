// Package handler 提供 HTTP 请求处理器
// 本文件处理 WebSocket 连接相关的 API 请求
package handler

import (
	"net/http"

	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service/chat"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// WsLoginHandler WebSocket 登录（升级 HTTP 连接为 WebSocket）
// GET /ws/login?client_id=xxx
// 查询参数: client_id - 用户 UUID
// 功能:
//   - 将 HTTP 连接升级为 WebSocket 连接
//   - 注册客户端到在线用户列表
//   - 开始监听消息收发
func WsLoginHandler(c *gin.Context) {
	// 获取客户端 ID（用户 UUID）
	clientId := c.Query("client_id")
	if clientId == "" {
		zap.L().Error("clientId获取失败")
		c.JSON(http.StatusOK, gin.H{
			"code": errorx.CodeInvalidParam,
			"msg":  "clientId获取失败",
		})
		return
	}
	// 初始化 WebSocket 客户端连接
	chat.NewClientInit(c, clientId)
}

// WsLogoutHandler WebSocket 登出
// POST /ws/logout
// 请求体: request.WsLogoutRequest
// 功能:
//   - 从在线用户列表中移除客户端
//   - 关闭 WebSocket 连接
func WsLogoutHandler(c *gin.Context) {
	var req request.WsLogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	// 登出客户端
	if err := chat.ClientLogout(req.OwnerId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
