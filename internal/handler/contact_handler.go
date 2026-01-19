// Package handler 提供 HTTP 请求处理器
// 本文件处理联系人相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// ContactHandler 联系人请求处理器
type ContactHandler struct {
	contactSvc service.ContactService
	groupSvc   service.GroupService
}

// NewContactHandler 创建联系人处理器实例
func NewContactHandler(contactSvc service.ContactService, groupSvc service.GroupService) *ContactHandler {
	return &ContactHandler{
		contactSvc: contactSvc,
		groupSvc:   groupSvc,
	}
}

// GetUserList 获取好友列表
// GET /contact/getUserList
// 从JWT上下文获取当前用户ID
// 响应: []respond.MyUserListRespond
func (h *ContactHandler) GetUserList(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	data, err := h.contactSvc.GetUserList(userId.(string))
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// LoadMyJoinedGroup 获取已加入的群组（排除自己创建的）
// GET /contact/loadMyJoinedGroup
// 从JWT上下文获取当前用户ID
// 响应: []respond.MyGroupListRespond
func (h *ContactHandler) LoadMyJoinedGroup(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	data, err := h.groupSvc.GetJoinedGroups(userId.(string))
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
// GetFriendInfo 获取好友详细信息
// GET /contact/getFriendInfo?friendId=xxx
// 查询参数: request.GetFriendInfoRequest
// 响应: respond.GetFriendInfoRespond
// 安全: 从JWT上下文获取当前用户ID，校验好友关系
func (h *ContactHandler) GetFriendInfo(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.GetFriendInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.contactSvc.GetFriendInfo(userId.(string), req.FriendId)
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
// GetGroupDetail 获取群聊详细信息
// GET /contact/getGroupDetail?groupId=xxx
// 查询参数: request.GetGroupInfoRequest
// 响应: respond.GetGroupDetailRespond
// 安全: 从JWT上下文获取当前用户ID，校验群成员身份
func (h *ContactHandler) GetGroupDetail(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.GetGroupInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.contactSvc.GetGroupDetail(userId.(string), req.GroupId)
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
// 安全: 从JWT上下文获取当前用户ID，防止IDOR攻击
func (h *ContactHandler) DeleteContact(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.DeleteContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.DeleteContact(userId.(string), req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackContact 拉黑联系人
// POST /contact/blackContact
// 请求体: request.BlackContactRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，防止IDOR攻击
func (h *ContactHandler) BlackContact(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.BlackContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.BlackContact(userId.(string), req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// CancelBlackContact 取消拉黑联系人
// POST /contact/cancelBlackContact
// 请求体: request.BlackContactRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，防止IDOR攻击
func (h *ContactHandler) CancelBlackContact(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.BlackContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.contactSvc.CancelBlackContact(userId.(string), req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
