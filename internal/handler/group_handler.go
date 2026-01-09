// Package handler 提供 HTTP 请求处理器
// 本文件处理群组相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// GroupHandler 群组请求处理器
// 通过构造函数注入 GroupService，遵循依赖倒置原则
type GroupHandler struct {
	groupSvc service.GroupService
}

// NewGroupHandler 创建群组处理器实例
// groupSvc: 群组服务接口
func NewGroupHandler(groupSvc service.GroupService) *GroupHandler {
	return &GroupHandler{groupSvc: groupSvc}
}

// CreateGroup 创建群聊
// POST /group/createGroup
// 请求体: request.CreateGroupRequest
// 响应: nil
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req request.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.CreateGroup(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// LoadMyGroup 获取我创建的群聊
// GET /group/loadMyGroup?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.LoadMyGroupRespond
func (h *GroupHandler) LoadMyGroup(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.groupSvc.LoadMyGroup(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// CheckGroupAddMode 检查群聊加入方式
// GET /group/checkGroupAddMode?groupId=xxx
// 查询参数: request.CheckGroupAddModeRequest
// 响应: int8 (0=直接加入, 1=需要审核)
func (h *GroupHandler) CheckGroupAddMode(c *gin.Context) {
	var req request.CheckGroupAddModeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	addMode, err := h.groupSvc.CheckGroupAddMode(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, addMode)
}

// EnterGroupDirectly 直接加入群聊（无需审核）
// POST /group/enterGroupDirectly
// 请求体: request.EnterGroupDirectlyRequest
// 响应: nil
func (h *GroupHandler) EnterGroupDirectly(c *gin.Context) {
	var req request.EnterGroupDirectlyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.EnterGroupDirectly(req.GroupId, req.UserId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// LeaveGroup 退出群聊
// POST /group/leaveGroup
// 请求体: request.LeaveGroupRequest
// 响应: nil
func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	var req request.LeaveGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.LeaveGroup(req.UserId, req.GroupId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// DismissGroup 解散群聊（仅群主可操作）
// POST /group/dismissGroup
// 请求体: request.DismissGroupRequest
// 响应: nil
func (h *GroupHandler) DismissGroup(c *gin.Context) {
	var req request.DismissGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.DismissGroup(req.OwnerId, req.GroupId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetGroupInfo 获取群聊详细信息
// GET /group/getGroupInfo?groupId=xxx
// 查询参数: request.GetGroupInfoRequest
// 响应: respond.GetGroupInfoRespond
func (h *GroupHandler) GetGroupInfo(c *gin.Context) {
	var req request.GetGroupInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.groupSvc.GetGroupInfo(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetGroupInfoList 获取群聊列表（管理员功能）
// GET /group/getGroupInfoList?page=1&pageSize=10
// 查询参数: request.GetGroupListRequest
// 响应: respond.GetGroupListWrapper
func (h *GroupHandler) GetGroupInfoList(c *gin.Context) {
	var req request.GetGroupListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.groupSvc.GetGroupInfoList(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// DeleteGroups 批量删除群聊（管理员功能）
// POST /group/deleteGroups
// 请求体: request.DeleteGroupsRequest
// 响应: nil
func (h *GroupHandler) DeleteGroups(c *gin.Context) {
	var req request.DeleteGroupsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.DeleteGroups(req.UuidList); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// SetGroupsStatus 批量设置群聊状态（管理员功能）
// POST /group/setGroupsStatus
// 请求体: request.SetGroupsStatusRequest
// 响应: nil
func (h *GroupHandler) SetGroupsStatus(c *gin.Context) {
	var req request.SetGroupsStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.SetGroupsStatus(req.UuidList, req.Status); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// UpdateGroupInfo 更新群聊信息
// POST /group/updateGroupInfo
// 请求体: request.UpdateGroupInfoRequest
// 响应: nil
func (h *GroupHandler) UpdateGroupInfo(c *gin.Context) {
	var req request.UpdateGroupInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.UpdateGroupInfo(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetGroupMemberList 获取群成员列表
// GET /group/getGroupMemberList?groupId=xxx
// 查询参数: request.GetGroupMemberListRequest
// 响应: []respond.GetGroupMemberListRespond
func (h *GroupHandler) GetGroupMemberList(c *gin.Context) {
	var req request.GetGroupMemberListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.groupSvc.GetGroupMemberList(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// RemoveGroupMembers 移除群成员
// POST /group/removeGroupMembers
// 请求体: request.RemoveGroupMembersRequest
// 响应: nil
func (h *GroupHandler) RemoveGroupMembers(c *gin.Context) {
	var req request.RemoveGroupMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.RemoveGroupMembers(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
