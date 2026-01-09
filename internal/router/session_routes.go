// Package router 提供 HTTP 路由注册
// 本文件定义会话相关的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterSessionRoutes 注册会话相关路由（需要认证）
// 包括会话的创建、查询、删除等功能
func (rt *Router) RegisterSessionRoutes(rg *gin.RouterGroup) {
	sessionGroup := rg.Group("/session")
	{
		sessionGroup.GET("/checkOpenSessionAllowed", rt.handlers.Session.CheckOpenSessionAllowed) // 检查是否允许打开会话
		sessionGroup.POST("/openSession", rt.handlers.Session.OpenSession)                        // 打开/创建会话
		sessionGroup.GET("/getUserSessionList", rt.handlers.Session.GetUserSessionList)           // 获取单聊会话列表
		sessionGroup.GET("/getGroupSessionList", rt.handlers.Session.GetGroupSessionList)         // 获取群聊会话列表
		sessionGroup.POST("/deleteSession", rt.handlers.Session.DeleteSession)                    // 删除会话
	}
}
