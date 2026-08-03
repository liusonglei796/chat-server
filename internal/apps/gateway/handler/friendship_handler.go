// Package handler 提供 HTTP 请求处理器
// 本文件处理好友关系相关的 API 请求
package handler

import (
	"kama_chat_server/internal/common/dto/request/friendship"
	"kama_chat_server/internal/common/grpc_client"
	relationpb "kama_chat_server/api/gen/relation"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// FriendshipHandler 好友关系请求处理器
type FriendshipHandler struct {
}

// NewFriendshipHandler 创建好友关系处理器实例
func NewFriendshipHandler() *FriendshipHandler {
	return &FriendshipHandler{}
}

// GetFriendList 获取好友列表（分页）
func (h *FriendshipHandler) GetFriendList(c *gin.Context) {
	ctx := c.Request.Context()

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

	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	rsp, err := grpc_client.RelationClient.GetFriendList(ctx, &relationpb.GetFriendListRequest{
		UserId:   userId.(string),
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, gin.H{
		"list":      rsp.List,
		"total":     rsp.Total,
		"page":      page,
		"page_size": pageSize,
	})
}

// LoadMyJoinedGroup 获取已加入的群组（分页）
func (h *FriendshipHandler) LoadMyJoinedGroup(c *gin.Context) {
	ctx := c.Request.Context()

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

	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	rsp, err := grpc_client.RelationClient.GetGroupListByMember(ctx, &relationpb.GetGroupListByMemberRequest{
		UserId:   userId.(string),
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, gin.H{
		"list":      rsp.List,
		"total":     rsp.Total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetFriendInfo 获取好友详细信息
func (h *FriendshipHandler) GetFriendInfo(c *gin.Context) {
	ctx := c.Request.Context()

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
	rsp, err := grpc_client.RelationClient.GetFriendInfo(ctx, &relationpb.GetFriendInfoRequest{
		UserId:   userId.(string),
		FriendId: req.FriendId,
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, rsp)
}

// DeleteFriend 删除好友
func (h *FriendshipHandler) DeleteFriend(c *gin.Context) {
	ctx := c.Request.Context()

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

	for _, uuid := range req.UuidList {
		if _, err := grpc_client.RelationClient.DeleteFriend(ctx, &relationpb.DeleteFriendRequest{
			UserId:   userId.(string),
			FriendId: uuid,
		}); err != nil {
			HandleError(c, err)
			return
		}
	}

	HandleSuccess(c, nil)
}

// BlockFriend 拉黑好友
func (h *FriendshipHandler) BlockFriend(c *gin.Context) {
	ctx := c.Request.Context()

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

	if _, err := grpc_client.RelationClient.BlackFriend(ctx, &relationpb.BlackFriendRequest{
		UserId:   userId.(string),
		FriendId: req.FriendId,
	}); err != nil {
		HandleError(c, err)
		return
	}

	HandleSuccess(c, nil)
}

// UnblockFriend 取消拉黑好友
func (h *FriendshipHandler) UnblockFriend(c *gin.Context) {
	ctx := c.Request.Context()

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

	if _, err := grpc_client.RelationClient.UnblackFriend(ctx, &relationpb.UnblackFriendRequest{
		UserId:   userId.(string),
		FriendId: req.FriendId,
	}); err != nil {
		HandleError(c, err)
		return
	}

	HandleSuccess(c, nil)
}

// UpdateRemark 更新好友备注
func (h *FriendshipHandler) UpdateRemark(c *gin.Context) {
	ctx := c.Request.Context()

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

	if _, err := grpc_client.RelationClient.UpdateRemark(ctx, &relationpb.UpdateRemarkRequest{
		UserId:   userId.(string),
		FriendId: req.FriendId,
		Remark:   req.Remark,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
