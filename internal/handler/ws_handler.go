package handler

import (
	"net/http"

	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/gateway/websocket"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// WsLogin wss登录 Get
func WsLoginHandler(c *gin.Context) {
	clientId := c.Query("client_id")
	if clientId == "" {
		zap.L().Error("clientId获取失败")
		c.JSON(http.StatusOK, gin.H{
			"code": errorx.CodeInvalidParam,
			"msg":  "clientId获取失败",
		})
		return
	}
	websocket.NewClientInit(c, clientId)
}

// WsLogout wss登出
func WsLogoutHandler(c *gin.Context) {
	var req request.WsLogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := websocket.ClientLogout(req.OwnerId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
