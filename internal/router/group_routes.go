// Package router 提供 HTTP 路由注册
// 本文件定义群组相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterGroupRoutes 注册群组相关路由（需要认证）
// 包括群组创建、管理、成员管理等功能
func RegisterGroupRoutes(rg *gin.RouterGroup) {
	groupGroup := rg.Group("/group")
	{
		// ===== 群组基本操作 =====
		groupGroup.POST("/createGroup", handler.CreateGroupHandler)            // 创建群组
		groupGroup.GET("/loadMyGroup", handler.LoadMyGroupHandler)             // 获取我创建的群组
		groupGroup.GET("/loadMyJoinedGroup", handler.LoadMyJoinedGroupHandler) // 获取已加入的群组
		groupGroup.GET("/getGroupInfo", handler.GetGroupInfoHandler)           // 获取群组详情
		groupGroup.GET("/getGroupDetail", handler.GetGroupDetailHandler)       // 获取群聊详情（会话用）
		groupGroup.POST("/updateGroupInfo", handler.UpdateGroupInfoHandler)    // 更新群组信息
		groupGroup.POST("/dismissGroup", handler.DismissGroupHandler)          // 解散群组（群主）

		//退群
		groupGroup.POST("/leaveGroup", handler.LeaveGroupHandler) // 退出群组

		// ===== 群成员管理 =====
		groupGroup.GET("/getGroupMemberList", handler.GetGroupMemberListHandler)  // 获取群成员列表
		groupGroup.POST("/removeGroupMembers", handler.RemoveGroupMembersHandler) // 移除群成员

		// 加群
		groupGroup.GET("/checkGroupAddMode", handler.CheckGroupAddModeHandler)    // 检查加群方式
		groupGroup.POST("/enterGroupDirectly", handler.EnterGroupDirectlyHandler) // 直接加入群组
		groupGroup.POST("/apply", handler.ApplyGroupHandler)                      // 需要申请加入群组
		groupGroup.GET("/applyList", handler.GetGroupApplyListHandler)            // 获取待处理的入群申请
		groupGroup.POST("/passApply", handler.PassGroupApplyHandler)              // 通过入群申请
		groupGroup.POST("/refuseApply", handler.RefuseGroupApplyHandler)          // 拒绝入群申请
		groupGroup.POST("/blackApply", handler.BlackGroupApplyHandler)            // 拉黑入群申请
	}
}
