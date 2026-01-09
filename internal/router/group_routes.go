// Package router 提供 HTTP 路由注册
// 本文件定义群组相关的路由
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterGroupRoutes 注册群组相关路由（需要认证）
// 包括群组创建、管理、成员管理等功能
func (rt *Router) RegisterGroupRoutes(rg *gin.RouterGroup) {
	groupGroup := rg.Group("/group")
	{
		// ===== 群组基本操作 =====
		groupGroup.POST("/createGroup", rt.handlers.Group.CreateGroup)              // 创建群组
		groupGroup.GET("/loadMyGroup", rt.handlers.Group.LoadMyGroup)               // 获取我创建的群组
		groupGroup.GET("/loadMyJoinedGroup", rt.handlers.Contact.LoadMyJoinedGroup) // 获取已加入的群组
		groupGroup.GET("/getGroupInfo", rt.handlers.Group.GetGroupInfo)             // 获取群组详情
		groupGroup.GET("/getGroupDetail", rt.handlers.Contact.GetGroupDetail)       // 获取群聊详情（会话用）
		groupGroup.POST("/updateGroupInfo", rt.handlers.Group.UpdateGroupInfo)      // 更新群组信息
		groupGroup.POST("/dismissGroup", rt.handlers.Group.DismissGroup)            // 解散群组（群主）

		//退群
		groupGroup.POST("/leaveGroup", rt.handlers.Group.LeaveGroup) // 退出群组

		// ===== 群成员管理 =====
		groupGroup.GET("/getGroupMemberList", rt.handlers.Group.GetGroupMemberList)  // 获取群成员列表
		groupGroup.POST("/removeGroupMembers", rt.handlers.Group.RemoveGroupMembers) // 移除群成员

		// 加群
		groupGroup.GET("/checkGroupAddMode", rt.handlers.Group.CheckGroupAddMode)    // 检查加群方式
		groupGroup.POST("/enterGroupDirectly", rt.handlers.Group.EnterGroupDirectly) // 直接加入群组
		groupGroup.POST("/apply", rt.handlers.Contact.ApplyGroup)                    // 需要申请加入群组
		groupGroup.GET("/applyList", rt.handlers.Contact.GetGroupApplyList)          // 获取待处理的入群申请
		groupGroup.POST("/passApply", rt.handlers.Contact.PassGroupApply)            // 通过入群申请
		groupGroup.POST("/refuseApply", rt.handlers.Contact.RefuseGroupApply)        // 拒绝入群申请
		groupGroup.POST("/blackApply", rt.handlers.Contact.BlackGroupApply)          // 拉黑入群申请
	}
}
