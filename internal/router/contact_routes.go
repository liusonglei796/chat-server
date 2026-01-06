// Package router 提供 HTTP 路由注册
// 本文件定义联系人相关的路由
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterContactRoutes 注册联系人相关路由
// 包括好友管理、联系人申请等功能
func RegisterContactRoutes(r *gin.Engine) {
	// 好友列表查询
	r.GET("/contact/getUserList", handler.GetUserListHandler)             // 获取好友列表
	r.GET("/contact/loadMyJoinedGroup", handler.LoadMyJoinedGroupHandler) // 获取已加入的群组
	r.GET("/contact/getContactInfo", handler.GetContactInfoHandler)       // 获取联系人详情

	// 好友关系管理
	r.POST("/contact/deleteContact", handler.DeleteContactHandler)           // 删除好友
	r.POST("/contact/blackContact", handler.BlackContactHandler)             // 拉黑好友
	r.POST("/contact/cancelBlackContact", handler.CancelBlackContactHandler) // 取消拉黑

	// 好友申请管理
	r.POST("/contact/applyContact", handler.ApplyContactHandler)             // 申请添加好友/入群
	r.GET("/contact/getNewContactList", handler.GetNewContactListHandler)    // 获取待处理的好友申请
	r.POST("/contact/passContactApply", handler.PassContactApplyHandler)     // 通过好友申请
	r.POST("/contact/refuseContactApply", handler.RefuseContactApplyHandler) // 拒绝好友申请
	r.POST("/contact/blackApply", handler.BlackApplyHandler)                 // 拉黑申请者

	// 入群申请管理
	r.GET("/contact/getAddGroupList", handler.GetAddGroupListHandler) // 获取待处理的入群申请
}
