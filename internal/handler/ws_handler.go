// Package handler 提供 HTTP 请求处理器
// 本文件处理 WebSocket 连接相关的 API 请求
package handler

import (
	"net/http"

	"kama_chat_server/internal/service/chat"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// WsHandler WebSocket 请求处理器
type WsHandler struct {
	broker chat.MessageBroker
}

// NewWsHandler 创建 WebSocket 处理器实例
func NewWsHandler(broker chat.MessageBroker) *WsHandler {
	return &WsHandler{broker: broker}
}

// WsLoginHandler WebSocket 登录（升级 HTTP 连接为 WebSocket）
// GET /ws/login
// 安全: 从JWT上下文获取用户ID，防止IDOR攻击
// 功能:
//   - 将 HTTP 连接升级为 WebSocket 连接
//   - 注册客户端到在线用户列表
//   - 开始监听消息收发
func (h *WsHandler) WsLoginHandler(c *gin.Context) {
	// 安全: 从 JWT 上下文获取用户 ID，不信任客户端传入的 client_id
	userId, exists := c.Get("user_id")
	if !exists {
		zap.L().Error("用户未认证，无法建立 WebSocket 连接")
		c.JSON(http.StatusOK, gin.H{
			"code": errorx.CodeUnauthorized,
			"msg":  "请先登录",
		})
		return
	}

	clientId := userId.(string)
	// 初始化 WebSocket 客户端连接
	chat.NewClientInit(c, clientId, h.broker)
}

// WsLogoutHandler WebSocket 登出
// POST /ws/logout
// 安全: 从JWT上下文获取用户ID，防止IDOR攻击
// 功能:
//   - 从在线用户列表中移除客户端
//   - 关闭 WebSocket 连接
func (h *WsHandler) WsLogoutHandler(c *gin.Context) {
	// 安全: 从 JWT 上下文获取用户 ID，不使用请求体中的 OwnerId
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	// 登出当前用户的客户端
	if err := chat.ClientLogout(userId.(string), h.broker); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
