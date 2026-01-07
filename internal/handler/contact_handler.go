// Package handler 提供 HTTP 请求处理器
// 本文件处理联系人相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// GetUserListHandler 获取好友列表
// GET /contact/getUserList?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.MyUserListRespond
func GetUserListHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetUserList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// LoadMyJoinedGroupHandler 获取已加入的群组（排除自己创建的）
// GET /contact/loadMyJoinedGroup?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.LoadMyJoinedGroupRespond
func LoadMyJoinedGroupHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetJoinedGroupsExcludedOwn(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetFriendInfoHandler 获取好友详细信息
// GET /contact/getFriendInfo?friendId=xxx
// 查询参数: request.GetFriendInfoRequest
// 响应: respond.GetFriendInfoRespond
func GetFriendInfoHandler(c *gin.Context) {
	var req request.GetFriendInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetFriendInfo(req.FriendId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetGroupDetailHandler 获取群聊详细信息
// GET /contact/getGroupDetail?groupId=xxx
// 查询参数: request.GetGroupInfoRequest
// 响应: respond.GetGroupDetailRespond
func GetGroupDetailHandler(c *gin.Context) {
	var req request.GetGroupInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetGroupDetail(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// DeleteContactHandler 删除联系人
// POST /contact/deleteContact
// 请求体: request.DeleteContactRequest
// 响应: nil
func DeleteContactHandler(c *gin.Context) {
	var req request.DeleteContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.DeleteContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// ApplyFriendHandler 申请添加好友
// POST /contact/applyFriend
// 请求体: request.ApplyFriendRequest
// 响应: nil
func ApplyFriendHandler(c *gin.Context) {
	var req request.ApplyFriendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.ApplyFriend(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// ApplyGroupHandler 申请加入群组
// POST /contact/applyGroup
// 请求体: request.ApplyGroupRequest
// 响应: nil
func ApplyGroupHandler(c *gin.Context) {
	var req request.ApplyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.ApplyGroup(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetFriendApplyListHandler 获取待处理的好友申请列表
// GET /contact/getFriendApplyList?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.NewContactListRespond
func GetFriendApplyListHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetFriendApplyList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetGroupApplyListHandler 获取入群申请列表
// GET /contact/getGroupApplyList?groupId=xxx
// 查询参数: request.AddGroupListRequest
// 响应: []respond.AddGroupListRespond
func GetGroupApplyListHandler(c *gin.Context) {
	var req request.AddGroupListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetGroupApplyList(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// PassFriendApplyHandler 通过好友申请
// POST /contact/passFriendApply
// 请求体: request.PassFriendApplyRequest
// 响应: nil
func PassFriendApplyHandler(c *gin.Context) {
	var req request.PassFriendApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.PassFriendApply(req.UserId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// PassGroupApplyHandler 通过入群申请
// POST /contact/passGroupApply
// 请求体: request.PassGroupApplyRequest
// 响应: nil
func PassGroupApplyHandler(c *gin.Context) {
	var req request.PassGroupApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.PassGroupApply(req.GroupId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// RefuseFriendApplyHandler 拒绝好友申请
// POST /contact/refuseFriendApply
// 请求体: request.PassFriendApplyRequest
// 响应: nil
func RefuseFriendApplyHandler(c *gin.Context) {
	var req request.PassFriendApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.RefuseFriendApply(req.UserId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// RefuseGroupApplyHandler 拒绝入群申请
// POST /contact/refuseGroupApply
// 请求体: request.PassGroupApplyRequest
// 响应: nil
func RefuseGroupApplyHandler(c *gin.Context) {
	var req request.PassGroupApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.RefuseGroupApply(req.GroupId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackContactHandler 拉黑联系人
// POST /contact/blackContact
// 请求体: request.BlackContactRequest
// 响应: nil
func BlackContactHandler(c *gin.Context) {
	var req request.BlackContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.BlackContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// CancelBlackContactHandler 取消拉黑联系人
// POST /contact/cancelBlackContact
// 请求体: request.BlackContactRequest
// 响应: nil
func CancelBlackContactHandler(c *gin.Context) {
	var req request.BlackContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.CancelBlackContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackFriendApplyHandler 拉黑好友申请
// POST /contact/blackFriendApply
// 请求体: request.BlackFriendApplyRequest
// 响应: nil
func BlackFriendApplyHandler(c *gin.Context) {
	var req request.BlackFriendApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.BlackFriendApply(req.UserId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackGroupApplyHandler 拉黑入群申请
// POST /contact/blackGroupApply
// 请求体: request.BlackGroupApplyRequest
// 响应: nil
func BlackGroupApplyHandler(c *gin.Context) {
	var req request.BlackGroupApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.BlackGroupApply(req.GroupId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
