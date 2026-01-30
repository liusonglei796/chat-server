// Package handler 提供 HTTP 请求处理器
// 本文件处理联系人相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request/contact"
	"kama_chat_server/internal/dto/request/group"
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

// GetUserList 获取好友列表（分页）
// GET /contact/getUserList?page=1&page_size=20
// 从JWT上下文获取当前用户ID
// 响应: map[string]interface{} (list, total, page, page_size)
func (h *ContactHandler) GetUserList(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req contact.GetFriendListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	// 设置默认值
	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	data, total, err := h.contactSvc.GetUserList(userId.(string), page, pageSize)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, gin.H{
		"list":      data,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// LoadMyJoinedGroup 获取已加入的群组（分页）
// GET /contact/loadMyJoinedGroup?page=1&page_size=20
// 从JWT上下文获取当前用户ID
// 响应: map[string]interface{} (list, total, page, page_size)
func (h *ContactHandler) LoadMyJoinedGroup(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req contact.GetJoinedGroupListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	// 设置默认值
	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	data, total, err := h.contactSvc.GetGroupList(userId.(string), page, pageSize)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, gin.H{
		"list":      data,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetFriendInfo 获取好友详细信息
// GET /contact/getFriendInfo?friendId=xxx
// 查询参数: contact.GetFriendInfoRequest
// 响应: respond.PublicUserInfoRespond
// GetFriendInfo 获取好友详细信息
// GET /contact/getFriendInfo?friendId=xxx
// 查询参数: contact.GetFriendInfoRequest
// 响应: respond.PublicUserInfoRespond
// 安全: 从JWT上下文获取当前用户ID，校验好友关系
func (h *ContactHandler) GetFriendInfo(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req contact.GetFriendInfoRequest
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
// 查询参数: group.GetGroupInfoRequest
// 响应: respond.GetGroupInfoRespond
// GetGroupDetail 获取群聊详细信息
// GET /contact/getGroupDetail?groupId=xxx
// 查询参数: group.GetGroupInfoRequest
// 响应: respond.GetGroupInfoRespond
// 安全: 从JWT上下文获取当前用户ID，校验群成员身份
func (h *ContactHandler) GetGroupDetail(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.GetGroupInfoRequest
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

// 请求体: contact.BatchDeleteRequest

// 响应: nil

// 安全: 从JWT上下文获取当前用户ID，防止IDOR攻击

func (h *ContactHandler) DeleteContact(c *gin.Context) {

	userId, exists := c.Get("user_id")

	if !exists {

		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))

		return

	}



	var req contact.BatchDeleteRequest



		if err := c.ShouldBindJSON(&req); err != nil {



			HandleParamError(c, err)



			return



		}



	



		// 从 UuidList 中取第一个（单个操作）



		if len(req.UuidList) == 0 {



			HandleError(c, errorx.New(errorx.CodeInvalidParam, "uuid_list 不能为空"))



			return



		}



		if err := h.contactSvc.DeleteContact(userId.(string), req.UuidList[0]); err != nil {



			HandleError(c, err)



			return



		}

	HandleSuccess(c, nil)

}



// BlockContact 拉黑联系人

// POST /contact/blockContact

// 请求体: contact.BlockContactRequest

// 响应: nil

func (h *ContactHandler) BlockContact(c *gin.Context) {

	userId, exists := c.Get("user_id")

	if !exists {

		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))

		return

	}



	var req contact.BlockContactRequest

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



// UnblockContact 取消拉黑联系人

// POST /contact/unblockContact

// 请求体: contact.BlockContactRequest

// 响应: nil

func (h *ContactHandler) UnblockContact(c *gin.Context) {

	userId, exists := c.Get("user_id")

	if !exists {

		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))

		return

	}



	var req contact.BlockContactRequest

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

// BlockContact 拉黑联系人
// POST /contact/blockContact
type BlockContactRequest contact.BlockContactRequest

// UnblockContact 取消拉黑联系人
// POST /contact/unblockContact
type UnblockContactRequest contact.BlockContactRequest
