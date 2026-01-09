// Package router 提供 HTTP 路由注册
// 本文件定义管理员相关的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes 注册管理员相关路由（需要认证）
// 这些接口只能由管理员调用
func (rt *Router) RegisterAdminRoutes(rg *gin.RouterGroup) {
	adminGroup := rg.Group("/admin")
	{
		// ===== 用户管理 =====
		userAdminGroup := adminGroup.Group("/user")
		{
			userAdminGroup.GET("/list", rt.handlers.User.GetUserInfoList)  // 获取所有用户列表
			userAdminGroup.POST("/setAdmin", rt.handlers.User.SetAdmin)    // 设置管理员
			userAdminGroup.POST("/able", rt.handlers.User.AbleUsers)       // 批量启用用户
			userAdminGroup.POST("/disable", rt.handlers.User.DisableUsers) // 批量禁用用户
			userAdminGroup.POST("/delete", rt.handlers.User.DeleteUsers)   // 批量删除用户
		}

		// ===== 群组管理 =====
		groupAdminGroup := adminGroup.Group("/group")
		{
			groupAdminGroup.GET("/list", rt.handlers.Group.GetGroupInfoList)      // 分页获取所遇群组列表
			groupAdminGroup.POST("/delete", rt.handlers.Group.DeleteGroups)       // 批量删除群组
			groupAdminGroup.POST("/setStatus", rt.handlers.Group.SetGroupsStatus) // 批量设置群组状态
		}
	}
}
