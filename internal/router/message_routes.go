// Package router 提供 HTTP 路由注册
// 本文件定义消息相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterMessageRoutes 注册消息相关路由（需要认证）
// 包括消息历史查询和文件上传功能
func RegisterMessageRoutes(rg *gin.RouterGroup) {
	messageGroup := rg.Group("/message")
	{
		messageGroup.GET("/getMessageList", handler.GetMessageListHandler)           // 获取私聊消息记录
		messageGroup.GET("/getGroupMessageList", handler.GetGroupMessageListHandler) // 获取群聊消息记录
		messageGroup.POST("/uploadAvatar", handler.UploadAvatarHandler)              // 上传用户头像
		messageGroup.POST("/uploadFile", handler.UploadFileHandler)                  // 上传聊天文件
	}
}
