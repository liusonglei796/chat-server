// Package router 提供 HTTP 路由注册
// 本文件定义会话相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterSessionRoutes 注册会话相关路由
// 包括会话的创建、查询、删除等功能
func RegisterSessionRoutes(r *gin.Engine) {
	r.POST("/session/openSession", handler.OpenSessionHandler)                        // 打开/创建会话
	r.GET("/session/getUserSessionList", handler.GetUserSessionListHandler)           // 获取单聊会话列表
	r.GET("/session/getGroupSessionList", handler.GetGroupSessionListHandler)         // 获取群聊会话列表
	r.POST("/session/deleteSession", handler.DeleteSessionHandler)                    // 删除会话
	r.GET("/session/checkOpenSessionAllowed", handler.CheckOpenSessionAllowedHandler) // 检查是否允许打开会话
}
