// Package handler 提供 HTTP 请求处理器
// 本文件处理联系人相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"
	"github.com/gin-gonic/gin"
)

// ContactHandler 联系人请求处理器
// 通过构造函数注入 ContactService，遵循依赖倒置原则
type ContactHandler struct {
	contactSvc service.ContactService
}

// NewContactHandler 创建联系人处理器实例
// contactSvc: 联系人服务接口
func NewContactHandler(contactSvc service.ContactService) *ContactHandler {
	return &ContactHandler{contactSvc: contactSvc}
}

// GetUserList 获取好友列表
// GET /contact/getUserList?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.MyUserListRespond
func (h *ContactHandler) GetUserList(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.contactSvc.GetUserList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// LoadMyJoinedGroup 获取已加入的群组（排除自己创建的）
// GET /contact/loadMyJoinedGroup?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.LoadMyJoinedGroupRespond
func (h *ContactHandler) LoadMyJoinedGroup(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.contactSvc.GetJoinedGroupsExcludedOwn(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetFriendInfo 获取好友详细信息
// GET /contact/getFriendInfo?friendId=xxx
// 查询参数: request.GetFriendInfoRequest
// 响应: respond.GetFriendInfoRespond
func (h *ContactHandler) GetFriendInfo(c *gin.Context) {
	var req request.GetFriendInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.contactSvc.GetFriendInfo(req.FriendId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetGroupDetail 获取群聊详细信息
// GET /contact/getGroupDetail?groupId=xxx
// 查询参数: request.GetGroupInfoRequest
// 响应: respond.GetGroupDetailRespond
func (h *ContactHandler) GetGroupDetail(c *gin.Context) {
	var req request.GetGroupInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.contactSvc.GetGroupDetail(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// DeleteContact 删除联系人
// POST /contact/deleteContact
// 请求体: request.DeleteContactRequest
// 响应: nil
func (h *ContactHandler) DeleteContact(c *gin.Context) {
	var req request.DeleteContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.DeleteContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// ApplyFriend 申请添加好友
// POST /contact/applyFriend
// 请求体: request.ApplyFriendRequest
// 响应: nil
func (h *ContactHandler) ApplyFriend(c *gin.Context) {
	var req request.ApplyFriendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.ApplyFriend(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// ApplyGroup 申请加入群组
// POST /contact/applyGroup
// 请求体: request.ApplyGroupRequest
// 响应: nil
func (h *ContactHandler) ApplyGroup(c *gin.Context) {
	var req request.ApplyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.ApplyGroup(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetFriendApplyList 获取待处理的好友申请列表
// GET /contact/getFriendApplyList?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.NewContactListRespond
func (h *ContactHandler) GetFriendApplyList(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.contactSvc.GetFriendApplyList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetGroupApplyList 获取入群申请列表
// GET /contact/getGroupApplyList?groupId=xxx
// 查询参数: request.AddGroupListRequest
// 响应: []respond.AddGroupListRespond
func (h *ContactHandler) GetGroupApplyList(c *gin.Context) {
	var req request.AddGroupListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.contactSvc.GetGroupApplyList(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// PassFriendApply 通过好友申请
// POST /contact/passFriendApply
// 请求体: request.PassFriendApplyRequest
// 响应: nil
func (h *ContactHandler) PassFriendApply(c *gin.Context) {
	var req request.PassFriendApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.PassFriendApply(req.UserId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// PassGroupApply 通过入群申请
// POST /contact/passGroupApply
// 请求体: request.PassGroupApplyRequest
// 响应: nil
func (h *ContactHandler) PassGroupApply(c *gin.Context) {
	var req request.PassGroupApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.PassGroupApply(req.GroupId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// RefuseFriendApply 拒绝好友申请
// POST /contact/refuseFriendApply
// 请求体: request.PassFriendApplyRequest
// 响应: nil
func (h *ContactHandler) RefuseFriendApply(c *gin.Context) {
	var req request.PassFriendApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.RefuseFriendApply(req.UserId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// RefuseGroupApply 拒绝入群申请
// POST /contact/refuseGroupApply
// 请求体: request.PassGroupApplyRequest
// 响应: nil
func (h *ContactHandler) RefuseGroupApply(c *gin.Context) {
	var req request.PassGroupApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.RefuseGroupApply(req.GroupId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackContact 拉黑联系人
// POST /contact/blackContact
// 请求体: request.BlackContactRequest
// 响应: nil
func (h *ContactHandler) BlackContact(c *gin.Context) {
	var req request.BlackContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.BlackContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// CancelBlackContact 取消拉黑联系人
// POST /contact/cancelBlackContact
// 请求体: request.BlackContactRequest
// 响应: nil
func (h *ContactHandler) CancelBlackContact(c *gin.Context) {
	var req request.BlackContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.CancelBlackContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackFriendApply 拉黑好友申请
// POST /contact/blackFriendApply
// 请求体: request.BlackFriendRequest
// 响应: nil
func (h *ContactHandler) BlackFriendApply(c *gin.Context) {
	var req request.BlackFriendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.BlackFriendApply(req.UserId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackGroupApply 拉黑入群申请
// POST /contact/blackGroupApply
// 请求体: request.BlackGroupRequest
// 响应: nil
func (h *ContactHandler) BlackGroupApply(c *gin.Context) {
	var req request.BlackGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.BlackGroupApply(req.GroupId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
