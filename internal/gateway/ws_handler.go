package gateway

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kama_chat_server/internal/infra/jwt"
	"kama_chat_server/pkg/errorx"
)

// WsHandler WebSocket 请求处理器
type WsHandler struct {
	hub *ConnHub
}

// NewWsHandler 创建 WebSocket 处理器实例
func NewWsHandler(hub *ConnHub) *WsHandler {
	return &WsHandler{hub: hub}
}

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
	userId := extractWsUserID(c)
	if userId == "" {
		zap.L().Error("用户未认证，无法建立 WebSocket 连接")
		c.JSON(http.StatusOK, gin.H{
			"code": errorx.CodeUnauthorized,
			"msg":  "请先登录",
		})
		return
	}

	NewClientInit(c, userId, h.hub)
}

// WsLogoutHandler WebSocket 登出
// POST /ws/logout
func (h *WsHandler) WsLogoutHandler(c *gin.Context) {
	userId := extractWsUserID(c)
	if userId == "" {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	if err := ClientLogout(userId, h.hub); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
