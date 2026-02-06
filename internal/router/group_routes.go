// Package router 提供 HTTP 路由注册
// 本文件定义群组相关的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterGroupRoutes 注册群组相关路由（需要认证）
// 使用 RESTful 风格：复数名词 + HTTP 动词语义
func (rt *Router) RegisterGroupRoutes(rg *gin.RouterGroup) {
	groupGroup := rg.Group("/groups")
	{
		// ===== 群组基本操作 =====
		groupGroup.POST("", rt.handlers.Group.CreateGroup)                       // 创建群组
		groupGroup.GET("/owned", rt.handlers.Group.LoadMyGroup)                  // 获取我创建的群组
		groupGroup.GET("/joined", rt.handlers.Friendship.LoadMyJoinedGroup)      // 获取已加入的群组
		groupGroup.GET("/detail", rt.handlers.Group.GetGroupDetail)         // 获取群聊详情
		groupGroup.PUT("/info", rt.handlers.Group.UpdateGroupInfo)               // 更新群组信息
		groupGroup.DELETE("", rt.handlers.Group.DismissGroup)                    // 解散群组（群主）
		groupGroup.POST("/leave", rt.handlers.Group.LeaveGroup)                  // 退出群组

		// ===== 群成员管理 =====
		groupGroup.GET("/members", rt.handlers.Group.GetGroupMemberList)         // 获取群成员列表
		groupGroup.DELETE("/members", rt.handlers.Group.RemoveGroupMembers)      // 移除群成员

		// ===== 加群 =====
		groupGroup.GET("/add-mode", rt.handlers.Group.CheckGroupAddMode)         // 检查加群方式
		groupGroup.POST("/apply", rt.handlers.Apply.ApplyGroup)                  // 申请加入群组
		groupGroup.GET("/applies", rt.handlers.Apply.GetGroupApplyList)          // 获取待处理的入群申请
		groupGroup.POST("/applies/approve", rt.handlers.Apply.PassGroupApply)    // 通过入群申请
		groupGroup.POST("/applies/refuse", rt.handlers.Apply.RefuseGroupApply)   // 拒绝入群申请
		groupGroup.POST("/applies/block", rt.handlers.Apply.BlackGroupApply)     // 拉黑入群申请
	}
}
