// Package router 提供 HTTP 路由注册
// 本文件定义好友相关的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterFriendRoutes 注册好友相关路由（需要认证）
// 包括好友列表查询、好友详情、好友关系管理等
func (rt *Router) RegisterFriendRoutes(rg *gin.RouterGroup) {
	friendGroup := rg.Group("/friend")
	{
		// ===== 查询 =====
		friendGroup.GET("/list", rt.handlers.Contact.GetUserList)   // 获取好友列表
		friendGroup.GET("/info", rt.handlers.Contact.GetFriendInfo) // 获取好友详情

		// ===== 好友关系管理 =====
		friendGroup.POST("/delete", rt.handlers.Contact.DeleteContact)           // 删除好友
		friendGroup.POST("/black", rt.handlers.Contact.BlackContact)             // 拉黑好友
		friendGroup.POST("/cancelBlack", rt.handlers.Contact.CancelBlackContact) // 取消拉黑

		// ===== 好友申请 =====
		friendGroup.POST("/apply", rt.handlers.Contact.ApplyFriend)             // 申请添加好友
		friendGroup.GET("/applyList", rt.handlers.Contact.GetFriendApplyList)   // 获取待处理的好友申请
		friendGroup.POST("/passApply", rt.handlers.Contact.PassFriendApply)     // 通过好友申请
		friendGroup.POST("/refuseApply", rt.handlers.Contact.RefuseFriendApply) // 拒绝好友申请
		friendGroup.POST("/blackApply", rt.handlers.Contact.BlackFriendApply)   // 拉黑好友申请
	}
}
