// Package handler 提供 HTTP 请求处理器
// 本文件处理好友关系相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request/friendship"
	"kama_chat_server/internal/service"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// FriendshipHandler 好友关系请求处理器
type FriendshipHandler struct {
	friendshipSvc service.FriendshipService
	groupSvc      service.GroupService
}

// NewFriendshipHandler 创建好友关系处理器实例
func NewFriendshipHandler(friendshipSvc service.FriendshipService, groupSvc service.GroupService) *FriendshipHandler {
	return &FriendshipHandler{
		friendshipSvc: friendshipSvc,
		groupSvc:      groupSvc,
	}
}

// GetFriendList 获取好友列表（分页）
// GET /friends?page=1&page_size=20
// 从JWT上下文获取当前用户ID
// 响应: map[string]interface{} (list, total, page, page_size)
func (h *FriendshipHandler) GetFriendList(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req friendship.GetFriendListRequest
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

	data, total, err := h.friendshipSvc.GetFriendList(userId.(string), page, pageSize)
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
// GET /groups/joined?page=1&page_size=20
// 从JWT上下文获取当前用户ID
// 响应: map[string]interface{} (list, total, page, page_size)
func (h *FriendshipHandler) LoadMyJoinedGroup(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req friendship.GetJoinedGroupListRequest
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

	// 通过 GroupService 查询（基于 GroupMember 表）
	data, total, err := h.groupSvc.GetGroupListByMember(userId.(string), page, pageSize)
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
// GET /friends/info?friend_id=xxx
// 查询参数: friendship.GetFriendInfoRequest
// 响应: friendshiprsp.FriendInfoRespond
// 安全: 从JWT上下文获取当前用户ID，校验好友关系
func (h *FriendshipHandler) GetFriendInfo(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req friendship.GetFriendInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.friendshipSvc.GetFriendInfo(userId.(string), req.FriendId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// DeleteFriend 删除好友
// DELETE /friends
// 请求体: friendship.BatchDeleteRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，防止IDOR攻击
func (h *FriendshipHandler) DeleteFriend(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req friendship.BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if len(req.UuidList) == 0 {
		HandleError(c, errorx.New(errorx.CodeInvalidParam, "uuid_list 不能为空"))
		return
	}

	// 批量删除：遍历所有好友
	for _, uuid := range req.UuidList {
		if err := h.friendshipSvc.DeleteFriend(userId.(string), uuid); err != nil {
			HandleError(c, err)
			return
		}
	}

	HandleSuccess(c, nil)
}

// BlockFriend 拉黑好友
// POST /friends/block
// 请求体: friendship.BlockFriendRequest
// 响应: nil
func (h *FriendshipHandler) BlockFriend(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req friendship.BlockFriendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if err := h.friendshipSvc.BlackFriend(userId.(string), req.FriendId); err != nil {
		HandleError(c, err)
		return
	}

	HandleSuccess(c, nil)
}

// UnblockFriend 取消拉黑好友
// DELETE /friends/block
// 请求体: friendship.BlockFriendRequest
// 响应: nil
func (h *FriendshipHandler) UnblockFriend(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req friendship.BlockFriendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if err := h.friendshipSvc.UnblackFriend(userId.(string), req.FriendId); err != nil {
		HandleError(c, err)
		return
	}

	HandleSuccess(c, nil)
}

// UpdateRemark 更新好友备注
// PUT /friends/remark
// 请求体: friendship.UpdateRemarkRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，Service层校验好友关系
func (h *FriendshipHandler) UpdateRemark(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req friendship.UpdateRemarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if err := h.friendshipSvc.UpdateRemark(userId.(string), req.FriendId, req.Remark); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
