// Package handler 提供 HTTP 请求处理器
// 本文件处理 WebSocket 连接相关的 API 请求与生命周期管理
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kama_chat_server/internal/apps/message/chat"
	"kama_chat_server/internal/common/infrastructure/jwt"
	"kama_chat_server/pkg/errorx"
)

// WsHandler WebSocket 请求处理器
type WsHandler struct {
	hub *chat.ConnHub
}

// NewWsHandler 创建 WebSocket 处理器实例
// 来源: 依赖注入传入的全局 ConnHub 长连接管理器
// 用于: 处理 WebSocket 握手登录与断开登出
func NewWsHandler(hub *chat.ConnHub) *WsHandler {
	return &WsHandler{hub: hub}
}

// extractWsUserID 提取 WebSocket 握手请求的用户身份
// 来源: APISIX 透传的 X-User-Id 请求头、Gin 上下文 user_id、或 Query 参数 token
// 用于: 校验连接发起人并作为 ConnHub 的唯一会话 key
func extractWsUserID(c *gin.Context) string {
	if uid := c.GetHeader("X-User-Id"); uid != "" {
		return uid
	}
	if val, exists := c.Get("user_id"); exists {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	if tokenStr := c.Query("token"); tokenStr != "" {
		if claims, err := jwt.ParseToken(tokenStr); err == nil && claims != nil {
			return claims.UserID
		}
	}
	return ""
}

// WsLoginHandler WebSocket 登录（升级 HTTP 连接为 WebSocket）
// GET /ws
func (h *WsHandler) WsLoginHandler(c *gin.Context) {
	// Step 1.1: 提取已认证的长连接用户身份
	// 来源: extractWsUserID 解析网关透传头、上下文或 URL token
	// 用于: Step 1.2 校验身份有效性
	userId := extractWsUserID(c)
	if userId == "" {
		zap.L().Error("用户未认证，无法建立 WebSocket 连接")
		c.JSON(http.StatusOK, gin.H{
			"code": errorx.CodeUnauthorized,
			"msg":  "请先登录",
		})
		return
	}

	// Step 1.2: 升级 HTTP 连接并初始化长连接会话
	// 来源: Step 1.1 提取的 userId 及注入的 ConnHub
	// 用于: 注册客户端到在线列表并启动心跳与读写协程
	chat.NewClientInit(c, userId, h.hub)
}

// WsLogoutHandler WebSocket 登出
// POST /ws/logout
func (h *WsHandler) WsLogoutHandler(c *gin.Context) {
	// Step 2.1: 提取登出用户的身份标识
	// 来源: extractWsUserID 解析网关透传头、上下文或 URL token
	// 用于: Step 2.2 注销长连接会话
	userId := extractWsUserID(c)
	if userId == "" {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	// Step 2.2: 从长连接管理器中剔除用户并关闭 Socket
	// 来源: Step 2.1 确认的 userId
	// 用于: 返回登出成功响应
	if err := chat.ClientLogout(userId, h.hub); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
