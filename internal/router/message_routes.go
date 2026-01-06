// Package router 提供 HTTP 路由注册
// 本文件定义消息相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterMessageRoutes 注册消息相关路由
// 包括消息历史查询和文件上传功能
func RegisterMessageRoutes(r *gin.Engine) {
	r.GET("/message/getMessageList", handler.GetMessageListHandler)           // 获取私聊消息记录
	r.GET("/message/getGroupMessageList", handler.GetGroupMessageListHandler) // 获取群聊消息记录
	r.POST("/message/uploadAvatar", handler.UploadAvatarHandler)              // 上传用户头像
	r.POST("/message/uploadFile", handler.UploadFileHandler)                  // 上传聊天文件
}
