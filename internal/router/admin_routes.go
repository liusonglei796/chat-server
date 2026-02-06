// Package router 提供 HTTP 路由注册
// 本文件定义管理员相关的路由
package router

import (
	"kama_chat_server/internal/infrastructure/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes 注册管理员相关路由（需要认证 + 管理员权限）
// 这些接口只能由管理员调用
// 安全: JWT Claims 快速拒绝 + 可选的实时查库校验（防止权限撤销后仍可访问）
func (rt *Router) RegisterAdminRoutes(rg *gin.RouterGroup) {
	adminGroup := rg.Group("/admin")
	// 应用管理员权限校验中间件（双重校验：JWT Claims + 实时查库）
	adminGroup.Use(middleware.AdminAuth(rt.adminChecker))
	{
		// ===== 用户管理 =====
		userAdminGroup := adminGroup.Group("/user")
		{
			userAdminGroup.GET("/list", rt.handlers.Admin.GetUserListPaged)              // 分页获取用户列表
			userAdminGroup.POST("/setAdmin", rt.handlers.Admin.SetAdmin)                 // 设置管理员
			userAdminGroup.POST("/batchStatus", rt.handlers.Admin.BatchUpdateUserStatus) // 批量操作用户状态
		}

		// ===== 群组管理 =====
		groupAdminGroup := adminGroup.Group("/group")
		{
			groupAdminGroup.GET("/list", rt.handlers.Admin.GetGroupInfoList)      // 分页获取所有群组列表
			groupAdminGroup.POST("/delete", rt.handlers.Admin.DeleteGroups)       // 批量删除群组
			groupAdminGroup.POST("/setStatus", rt.handlers.Admin.SetGroupsStatus) // 批量设置群组状态
		}
	}
}
