// Package router 提供 HTTP 路由注册
// 本文件定义群组相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterGroupRoutes 注册群组相关路由
// 包括群组创建、管理、成员管理等功能
func RegisterGroupRoutes(r *gin.Engine) {
	// 群组基本操作
	r.POST("/group/createGroup", handler.CreateGroupHandler)         // 创建群组
	r.GET("/group/loadMyGroup", handler.LoadMyGroupHandler)          // 获取我创建的群组
	r.GET("/group/getGroupInfo", handler.GetGroupInfoHandler)        // 获取群组详情
	r.POST("/group/updateGroupInfo", handler.UpdateGroupInfoHandler) // 更新群组信息
	r.POST("/group/dismissGroup", handler.DismissGroupHandler)       // 解散群组（群主）

	// 加入/退出群组
	r.GET("/group/checkGroupAddMode", handler.CheckGroupAddModeHandler)    // 检查加群方式
	r.POST("/group/enterGroupDirectly", handler.EnterGroupDirectlyHandler) // 直接加入群组
	r.POST("/group/leaveGroup", handler.LeaveGroupHandler)                 // 退出群组

	// 群成员管理
	r.GET("/group/getGroupMemberList", handler.GetGroupMemberListHandler)  // 获取群成员列表
	r.POST("/group/removeGroupMembers", handler.RemoveGroupMembersHandler) // 移除群成员

	// 管理员功能
	r.GET("/group/getGroupInfoList", handler.GetGroupInfoListHandler) // 分页获取群组列表
	r.POST("/group/deleteGroups", handler.DeleteGroupsHandler)        // 批量删除群组
	r.POST("/group/setGroupsStatus", handler.SetGroupsStatusHandler)  // 批量设置群组状态
}
