// Package router 提供 HTTP 路由注册
// 本文件定义管理员相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes 注册管理员相关路由（需要认证）
// 这些接口只能由管理员调用
func RegisterAdminRoutes(rg *gin.RouterGroup) {
	adminGroup := rg.Group("/admin")
	{
		// ===== 用户管理 =====
		userAdminGroup := adminGroup.Group("/user")
		{
			userAdminGroup.GET("/list", handler.GetUserInfoListHandler)  // 获取所有用户列表
			userAdminGroup.POST("/setAdmin", handler.SetAdminHandler)    // 设置管理员
			userAdminGroup.POST("/able", handler.AbleUsersHandler)       // 批量启用用户
			userAdminGroup.POST("/disable", handler.DisableUsersHandler) // 批量禁用用户
			userAdminGroup.POST("/delete", handler.DeleteUsersHandler)   // 批量删除用户
		}

		// ===== 群组管理 =====
		groupAdminGroup := adminGroup.Group("/group")
		{
			groupAdminGroup.GET("/list", handler.GetGroupInfoListHandler)      // 分页获取所遇群组列表
			groupAdminGroup.POST("/delete", handler.DeleteGroupsHandler)       // 批量删除群组
			groupAdminGroup.POST("/setStatus", handler.SetGroupsStatusHandler) // 批量设置群组状态
		}
	}
}
