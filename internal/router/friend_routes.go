// Package router 提供 HTTP 路由注册
// 本文件定义好友相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterFriendRoutes 注册好友相关路由（需要认证）
// 包括好友列表查询、好友详情、好友关系管理等
func RegisterFriendRoutes(rg *gin.RouterGroup) {
	friendGroup := rg.Group("/friend")
	{
		// ===== 查询 =====
		friendGroup.GET("/list", handler.GetUserListHandler)   // 获取好友列表
		friendGroup.GET("/info", handler.GetFriendInfoHandler) // 获取好友详情

		// ===== 好友关系管理 =====
		friendGroup.POST("/delete", handler.DeleteContactHandler)           // 删除好友
		friendGroup.POST("/black", handler.BlackContactHandler)             // 拉黑好友
		friendGroup.POST("/cancelBlack", handler.CancelBlackContactHandler) // 取消拉黑

		// ===== 好友申请 =====
		friendGroup.POST("/apply", handler.ApplyFriendHandler)             // 申请添加好友
		friendGroup.GET("/applyList", handler.GetFriendApplyListHandler)   // 获取待处理的好友申请
		friendGroup.POST("/passApply", handler.PassFriendApplyHandler)     // 通过好友申请
		friendGroup.POST("/refuseApply", handler.RefuseFriendApplyHandler) // 拒绝好友申请
		friendGroup.POST("/blackApply", handler.BlackFriendApplyHandler)   // 拉黑好友申请
	}
}
