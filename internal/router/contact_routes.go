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
	r.GET("/contact/getFriendInfo", handler.GetFriendInfoHandler)         // 获取好友详情
	r.GET("/contact/getGroupDetail", handler.GetGroupDetailHandler)       // 获取群聊详情

	// 好友关系管理
	r.POST("/contact/deleteContact", handler.DeleteContactHandler)           // 删除好友
	r.POST("/contact/blackContact", handler.BlackContactHandler)             // 拉黑好友
	r.POST("/contact/cancelBlackContact", handler.CancelBlackContactHandler) // 取消拉黑

	// ===== 好友申请管理 =====
	r.POST("/contact/applyFriend", handler.ApplyFriendHandler)              // 申请添加好友
	r.GET("/contact/getFriendApplyList", handler.GetFriendApplyListHandler) // 获取待处理的好友申请
	r.POST("/contact/passFriendApply", handler.PassFriendApplyHandler)      // 通过好友申请
	r.POST("/contact/refuseFriendApply", handler.RefuseFriendApplyHandler)  // 拒绝好友申请
	r.POST("/contact/blackFriendApply", handler.BlackFriendApplyHandler)    // 拉黑好友申请

	// ===== 入群申请管理 =====
	r.POST("/contact/applyGroup", handler.ApplyGroupHandler)              // 申请加入群组
	r.GET("/contact/getGroupApplyList", handler.GetGroupApplyListHandler) // 获取待处理的入群申请
	r.POST("/contact/passGroupApply", handler.PassGroupApplyHandler)      // 通过入群申请
	r.POST("/contact/refuseGroupApply", handler.RefuseGroupApplyHandler)  // 拒绝入群申请
	r.POST("/contact/blackGroupApply", handler.BlackGroupApplyHandler)    // 拉黑入群申请
}
