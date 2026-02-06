// Package router 提供 HTTP 路由注册
// 本文件定义好友相关的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterFriendRoutes 注册好友相关路由（需要认证）
// 使用 RESTful 风格：复数名词 + HTTP 动词语义
func (rt *Router) RegisterFriendRoutes(rg *gin.RouterGroup) {
	friendGroup := rg.Group("/friends")
	{
		// ===== 查询 =====
		friendGroup.GET("", rt.handlers.Friendship.GetFriendList)     // 获取好友列表
		friendGroup.GET("/info", rt.handlers.Friendship.GetFriendInfo) // 获取好友详情

		// ===== 好友关系管理 =====
		friendGroup.DELETE("", rt.handlers.Friendship.DeleteFriend)         // 删除好友
		friendGroup.POST("/block", rt.handlers.Friendship.BlockFriend)      // 拉黑好友
		friendGroup.DELETE("/block", rt.handlers.Friendship.UnblockFriend)   // 取消拉黑

		// ===== 好友申请 =====
		friendGroup.POST("/apply", rt.handlers.Apply.ApplyFriend)                // 申请添加好友
		friendGroup.GET("/applies", rt.handlers.Apply.GetFriendApplyList)        // 获取待处理的好友申请
		friendGroup.POST("/applies/approve", rt.handlers.Apply.PassFriendApply)  // 通过好友申请
		friendGroup.POST("/applies/refuse", rt.handlers.Apply.RefuseFriendApply) // 拒绝好友申请
		friendGroup.POST("/applies/block", rt.handlers.Apply.BlackFriendApply)   // 拉黑好友申请
	}
}
