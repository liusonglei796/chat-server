// Package router 提供 HTTP 路由注册
// 本文件定义消息相关的路由和文件上传路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterMessageRoutes 注册消息相关路由（需要认证）
// 消息查询使用 /messages 前缀，文件上传使用 /upload 前缀（职责分离）
func (rt *Router) RegisterMessageRoutes(rg *gin.RouterGroup) {
	// ===== 消息查询 =====
	messageGroup := rg.Group("/messages")
	{
		messageGroup.GET("", rt.handlers.Message.GetMessageList)        // 获取聊天记录（私聊/群聊统一）
		messageGroup.POST("/recall", rt.handlers.Message.RecallMessage) // 撤回消息
		messageGroup.POST("/translate", rt.handlers.Message.Translate)  // 翻译消息
	}

	// ===== 文件上传（独立前缀） =====
	uploadGroup := rg.Group("/upload")
	{
		uploadGroup.POST("/avatar", rt.handlers.Message.UploadAvatar) // 上传用户头像
		uploadGroup.POST("/file", rt.handlers.Message.UploadFile)     // 上传聊天文件
	}
}
