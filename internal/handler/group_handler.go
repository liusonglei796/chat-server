// Package handler 提供 HTTP 请求处理器
// 本文件处理群组相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"
	"kama_chat_server/pkg/errorx"

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
// 安全: 从JWT上下文获取当前用户ID作为群主
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	ownerId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.CreateGroup(ownerId.(string), req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// LoadMyGroup 获取我创建的群聊
// GET /group/loadMyGroup
// 从JWT上下文获取当前用户ID
// 响应: []respond.MyGroupListRespond
func (h *GroupHandler) LoadMyGroup(c *gin.Context) {
	// 从JWT中间件获取当前用户ID
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	data, err := h.groupSvc.LoadMyGroup(userId.(string))
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

// LeaveGroup 退出群聊
// POST /group/leaveGroup
// 请求体: request.LeaveGroupRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，防止IDOR攻击
func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.LeaveGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.LeaveGroup(userId.(string), req.GroupId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// DismissGroup 解散群聊（仅群主可操作）
// POST /group/dismissGroup
// 请求体: request.DismissGroupRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，Service层校验群主权限
func (h *GroupHandler) DismissGroup(c *gin.Context) {
	operatorId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.DismissGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.DismissGroup(operatorId.(string), req.GroupId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// UpdateGroupInfo 更新群聊信息
// POST /group/updateGroupInfo
// 请求体: request.UpdateGroupInfoRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，Service层校验管理员权限
func (h *GroupHandler) UpdateGroupInfo(c *gin.Context) {
	operatorId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.UpdateGroupInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.UpdateGroupInfo(operatorId.(string), req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetGroupMemberList 获取群成员列表
// GET /group/getGroupMemberList?groupId=xxx
// 查询参数: request.GetGroupMemberListRequest
// 响应: []respond.GetGroupMemberListRespond
// 安全: 从JWT上下文获取当前用户ID，Service层校验成员身份
func (h *GroupHandler) GetGroupMemberList(c *gin.Context) {
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.GetGroupMemberListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.groupSvc.GetGroupMemberList(userId.(string), req.GroupId)
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
// 安全: 从JWT上下文获取当前用户ID，Service层校验管理员权限
func (h *GroupHandler) RemoveGroupMembers(c *gin.Context) {
	operatorId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req request.RemoveGroupMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.RemoveGroupMembers(operatorId.(string), req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
