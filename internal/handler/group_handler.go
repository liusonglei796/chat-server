// Package handler 提供 HTTP 请求处理器
// 本文件处理群组相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request/group"
	groupsvc "kama_chat_server/internal/service/group"
	"kama_chat_server/pkg/errorx"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GroupHandler 群组请求处理器
// 通过构造函数注入 GroupService，遵循依赖倒置原则
type GroupHandler struct {
	groupSvc *groupsvc.GroupService
}

// NewGroupHandler 创建群组处理器实例
// groupSvc: 群组服务
func NewGroupHandler(groupSvc *groupsvc.GroupService) *GroupHandler {
	return &GroupHandler{groupSvc: groupSvc}
}

// CreateGroup 创建群聊
// POST /group/createGroup
// 请求体: group.CreateGroupRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID作为群主
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	ctx := c.Request.Context()

	ownerId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.CreateGroup(ctx, ownerId.(string), req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// LoadMyGroup 获取我创建的群聊
// GET /group/loadMyGroup?page=1&page_size=20
// 从JWT上下文获取当前用户ID
// 响应: map[string]interface{} (list, total, page, page_size)
func (h *GroupHandler) LoadMyGroup(c *gin.Context) {
	ctx := c.Request.Context()

	// 从JWT中间件获取当前用户ID
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	// 解析分页参数
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

	data, total, err := h.groupSvc.LoadMyGroup(ctx, userId.(string), page, pageSize)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, map[string]interface{}{
		"list":      data,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// CheckGroupAddMode 检查群聊加入方式
// GET /group/checkGroupAddMode?groupId=xxx
// 查询参数: group.CheckGroupAddModeRequest
// 响应: int8 (0=直接加入, 1=需要审核)
func (h *GroupHandler) CheckGroupAddMode(c *gin.Context) {
	ctx := c.Request.Context()

	var req group.CheckGroupAddModeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	addMode, err := h.groupSvc.CheckGroupAddMode(ctx, req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, addMode)
}

// LeaveGroup 退出群聊
// POST /group/leaveGroup
// 请求体: group.LeaveGroupRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，防止IDOR攻击
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
	if err := h.groupSvc.LeaveGroup(ctx, userId.(string), req.GroupId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// DismissGroup 解散群聊（仅群主可操作）
// POST /group/dismissGroup
// 请求体: group.DismissGroupRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，Service层校验群主权限
func (h *GroupHandler) DismissGroup(c *gin.Context) {
	ctx := c.Request.Context()

	operatorId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.DismissGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.DismissGroup(ctx, operatorId.(string), req.GroupId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// UpdateGroupInfo 更新群聊信息
// POST /group/updateGroupInfo
// 请求体: group.UpdateGroupInfoRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，Service层校验管理员权限
func (h *GroupHandler) UpdateGroupInfo(c *gin.Context) {
	ctx := c.Request.Context()

	operatorId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.UpdateGroupInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.UpdateGroupInfo(ctx, operatorId.(string), req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetGroupMemberList 获取群成员列表
// GET /group/getGroupMemberList?group_id=xxx&page=1&page_size=20
// 查询参数: request.GetGroupMemberListRequest
// 响应: map[string]interface{} (list, total, page, page_size)
// 安全: 从JWT上下文获取当前用户ID，Service层校验成员身份
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

	// 设置默认分页参数
	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	data, total, err := h.groupSvc.GetGroupMemberList(ctx, userId.(string), req.GroupId, page, pageSize)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, map[string]interface{}{
		"list":      data,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// RemoveGroupMembers 移除群成员
// POST /group/removeGroupMembers
// 请求体: group.RemoveGroupMembersRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，Service层校验管理员权限
func (h *GroupHandler) RemoveGroupMembers(c *gin.Context) {
	ctx := c.Request.Context()

	operatorId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.RemoveGroupMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.RemoveGroupMembers(ctx, operatorId.(string), req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetGroupDetail 获取群聊详细信息
// GET /groups/detail?group_id=xxx
// 查询参数: group.GetGroupInfoRequest
// 响应: grouprsp.PublicGroupInfoRespond
// 安全: 从JWT上下文获取当前用户ID，Service层校验群成员身份
func (h *GroupHandler) GetGroupDetail(c *gin.Context) {
	ctx := c.Request.Context()

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
	data, err := h.groupSvc.GetGroupDetail(ctx, userId.(string), req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// MuteMember 禁言/取消禁言群成员
// POST /groups/members/mute
// 请求体: group.MuteMemberRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，Service层校验管理员权限
func (h *GroupHandler) MuteMember(c *gin.Context) {
	ctx := c.Request.Context()

	operatorId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req group.MuteMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}

	if err := h.groupSvc.MuteMember(ctx, operatorId.(string), req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
