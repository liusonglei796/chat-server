package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterMessageRoutes 注册消息相关路由
func RegisterMessageRoutes(r *gin.Engine) {
	r.GET("/message/getMessageList", handler.GetMessageListHandler)
	r.GET("/message/getGroupMessageList", handler.GetGroupMessageListHandler)
	r.POST("/message/uploadAvatar", handler.UploadAvatarHandler)
	r.POST("/message/uploadFile", handler.UploadFileHandler)
}
