// Package router 提供 HTTP 路由注册
// 本文件定义会话相关的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterSessionRoutes 注册会话相关路由（需要认证）
// 使用 RESTful 风格：复数名词 + HTTP 动词语义
func (rt *Router) RegisterSessionRoutes(rg *gin.RouterGroup) {
	sessionGroup := rg.Group("/sessions")
	{
		sessionGroup.GET("/check", rt.handlers.Session.CheckOpenSessionAllowed) // 检查是否允许打开会话
		sessionGroup.POST("", rt.handlers.Session.OpenSession)                  // 打开/创建会话
		sessionGroup.GET("/direct", rt.handlers.Session.GetUserSessionList)     // 获取单聊会话列表
		sessionGroup.GET("/group", rt.handlers.Session.GetGroupSessionList)     // 获取群聊会话列表
		sessionGroup.DELETE("", rt.handlers.Session.DeleteSession)              // 删除会话
		sessionGroup.PUT("/pin", rt.handlers.Session.PinSession)                // 置顶/取消置顶会话
	}
}
