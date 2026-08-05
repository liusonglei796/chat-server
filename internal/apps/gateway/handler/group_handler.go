// Package handler 提供 HTTP 请求处理器
// 本文件处理群组相关的 API 请求
package handler

import (
	"kama_chat_server/internal/common/dto/request/group"
	"kama_chat_server/internal/common/grpc_client"
	grouppb "kama_chat_server/api/gen/group"
	"kama_chat_server/pkg/errorx"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GroupHandler 群组请求处理器
type GroupHandler struct {
}

// NewGroupHandler 创建群组处理器实例
func NewGroupHandler() *GroupHandler {
	return &GroupHandler{}
}

// CreateGroup 创建群组
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if _, err := grpc_client.GroupClient.CreateGroup(ctx, &grouppb.CreateGroupRequest{
		OwnerId: userId.(string),
		Name:    req.Name,
		Notice:  req.Notice,
		AddMode: int32(req.AddMode),
		Avatar:  req.Avatar,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// LoadMyGroup 获取我创建的群组（分页）
func (h *GroupHandler) LoadMyGroup(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		if pVal, err := strconv.Atoi(p); err == nil && pVal > 0 {
			page = pVal
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if psVal, err := strconv.Atoi(ps); err == nil && psVal > 0 {
			pageSize = psVal
		}
	}

	rsp, err := grpc_client.GroupClient.LoadMyGroup(ctx, &grouppb.LoadMyGroupRequest{
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

// LeaveGroup 退出群组
func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.LeaveGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if _, err := grpc_client.GroupClient.LeaveGroup(ctx, &grouppb.LeaveGroupRequest{
		UserId:  userId.(string),
		GroupId: req.GroupId,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// DismissGroup 解散群组
func (h *GroupHandler) DismissGroup(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.DismissGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if _, err := grpc_client.GroupClient.DismissGroup(ctx, &grouppb.DismissGroupRequest{
		OperatorId: userId.(string),
		GroupId:    req.GroupId,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// UpdateGroupInfo 更新群组信息
func (h *GroupHandler) UpdateGroupInfo(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.UpdateGroupInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	rpcReq := &grouppb.UpdateGroupInfoRequest{
		OperatorId: userId.(string),
		Uuid:       req.Uuid,
		Name:       req.Name,
		Notice:     req.Notice,
		Avatar:     req.Avatar,
	}
	if req.AddMode != nil {
		v := int32(*req.AddMode)
		rpcReq.AddMode = &v
	}

	if _, err := grpc_client.GroupClient.UpdateGroupInfo(ctx, rpcReq); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetGroupMemberList 获取群成员列表
func (h *GroupHandler) GetGroupMemberList(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.GetGroupMemberListRequest
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

	rsp, err := grpc_client.GroupClient.GetGroupMemberList(ctx, &grouppb.GetGroupMemberListRequest{
		UserId:   userId.(string),
		GroupId:  req.GroupId,
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

// RemoveGroupMembers 踢出群成员
func (h *GroupHandler) RemoveGroupMembers(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.RemoveGroupMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if _, err := grpc_client.GroupClient.RemoveGroupMembers(ctx, &grouppb.RemoveGroupMembersRequest{
		OperatorId: userId.(string),
		GroupId:    req.GroupId,
		UuidList:   req.UuidList,
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// CheckGroupAddMode 检查群组添加模式
func (h *GroupHandler) CheckGroupAddMode(c *gin.Context) {
	groupId := c.Query("group_id")
	if groupId == "" {
		HandleError(c, errorx.New(errorx.CodeInvalidParam, "group_id不能为空"))
		return
	}
	rsp, err := grpc_client.GroupClient.CheckGroupAddMode(c.Request.Context(), &grouppb.CheckGroupAddModeRequest{
		GroupId: groupId,
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, gin.H{"add_mode": rsp.AddMode})
}

// GetGroupDetail 获取群组详细信息
func (h *GroupHandler) GetGroupDetail(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	groupId := c.Query("group_id")
	if groupId == "" {
		HandleError(c, errorx.New(errorx.CodeInvalidParam, "group_id 不能为空"))
		return
	}
	rsp, err := grpc_client.GroupClient.GetGroupDetail(ctx, &grouppb.GetGroupDetailRequest{
		UserId:  userId.(string),
		GroupId: groupId,
	})
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, rsp)
}

// MuteMember 禁言/取消禁言群成员
func (h *GroupHandler) MuteMember(c *gin.Context) {
	ctx := c.Request.Context()

	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.MuteMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if _, err := grpc_client.GroupClient.MuteMember(ctx, &grouppb.MuteMemberRequest{
		OperatorId: userId.(string),
		GroupId:    req.GroupId,
		UserId:     req.UserId,
		Duration:   int64(req.Duration),
	}); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
