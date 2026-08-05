// Package handler 提供 HTTP 请求处理器
// 本文件处理申请（好友/入群）相关的 API 请求
package handler

import (
	"kama_chat_server/internal/common/dto/request/apply"
	"kama_chat_server/internal/common/grpc_client"
	applypb "kama_chat_server/api/gen/apply"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// ApplyHandler 申请请求处理器
type ApplyHandler struct {
}

// NewApplyHandler 创建申请处理器实例
func NewApplyHandler() *ApplyHandler {
	return &ApplyHandler{}
}

// ApplyFriend 申请添加好友
func (h *ApplyHandler) ApplyFriend(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.ApplyFriendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if _, err := grpc_client.ApplyClient.ApplyFriend(ctx, &applypb.ApplyFriendRequest{
		UserId:   userId.(string),
		FriendId: req.FriendId,
		Message:  req.Message,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// ApplyGroup 申请加入群组
func (h *ApplyHandler) ApplyGroup(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.ApplyGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if _, err := grpc_client.ApplyClient.ApplyGroup(ctx, &applypb.ApplyGroupRequest{
		UserId:  userId.(string),
		GroupId: req.GroupId,
		Message: req.Message,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetFriendApplyList 获取好友申请列表（分页）
func (h *ApplyHandler) GetFriendApplyList(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.GetFriendApplyListRequest
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

	rsp, err := grpc_client.ApplyClient.GetFriendApplyList(ctx, &applypb.GetFriendApplyListRequest{
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

// GetGroupApplyList 获取群组申请列表（分页）
func (h *ApplyHandler) GetGroupApplyList(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.GetGroupApplyListRequest
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

	rsp, err := grpc_client.ApplyClient.GetGroupApplyList(ctx, &applypb.GetGroupApplyListRequest{
		OperatorId: userId.(string),
		GroupId:    req.GroupId,
		Page:       int32(page),
		PageSize:   int32(pageSize),
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

// PassFriendApply 同意好友申请
func (h *ApplyHandler) PassFriendApply(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.HandleFriendApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if _, err := grpc_client.ApplyClient.PassFriendApply(ctx, &applypb.PassFriendApplyRequest{
		UserId:      userId.(string),
		ApplicantId: req.ApplicantId,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// RefuseFriendApply 拒绝好友申请
func (h *ApplyHandler) RefuseFriendApply(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.HandleFriendApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if _, err := grpc_client.ApplyClient.RefuseFriendApply(ctx, &applypb.RefuseFriendApplyRequest{
		UserId:      userId.(string),
		ApplicantId: req.ApplicantId,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackFriendApply 拉黑好友申请
func (h *ApplyHandler) BlackFriendApply(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.HandleFriendApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if _, err := grpc_client.ApplyClient.BlackFriendApply(ctx, &applypb.BlackFriendApplyRequest{
		UserId:      userId.(string),
		ApplicantId: req.ApplicantId,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// PassGroupApply 同意入群申请
func (h *ApplyHandler) PassGroupApply(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.HandleGroupApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if _, err := grpc_client.ApplyClient.PassGroupApply(ctx, &applypb.PassGroupApplyRequest{
		OperatorId:  userId.(string),
		GroupId:     req.GroupId,
		ApplicantId: req.ApplicantId,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// RefuseGroupApply 拒绝入群申请
func (h *ApplyHandler) RefuseGroupApply(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.HandleGroupApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if _, err := grpc_client.ApplyClient.RefuseGroupApply(ctx, &applypb.RefuseGroupApplyRequest{
		OperatorId:  userId.(string),
		GroupId:     req.GroupId,
		ApplicantId: req.ApplicantId,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackGroupApply 拉黑入群申请
func (h *ApplyHandler) BlackGroupApply(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.HandleGroupApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if _, err := grpc_client.ApplyClient.BlackGroupApply(ctx, &applypb.BlackGroupApplyRequest{
		OperatorId:  userId.(string),
		GroupId:     req.GroupId,
		ApplicantId: req.ApplicantId,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
