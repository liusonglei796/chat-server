// Package router 提供 HTTP 路由注册
// 本文件定义 AI 相关路由
package router

import "github.com/gin-gonic/gin"

// RegisterAIRoutes 注册 AI 相关路由（需要认证）
func (rt *Router) RegisterAIRoutes(rg *gin.RouterGroup) {
	aiGroup := rg.Group("/ai")
	{
		aiGroup.POST("/reply-suggestions", rt.handlers.AI.ReplySuggestions)
		aiGroup.POST("/group-summary", rt.handlers.AI.GroupSummary)
		aiGroup.POST("/translate", rt.handlers.AI.Translate)
	}
}
