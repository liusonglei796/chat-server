// Package handler 提供 HTTP 请求处理器
// 本文件处理群组相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// CreateGroupHandler 创建群聊
// POST /group/createGroup
// 请求体: request.CreateGroupRequest
// 响应: nil
func CreateGroupHandler(c *gin.Context) {
	var req request.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.CreateGroup(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// LoadMyGroupHandler 获取我创建的群聊
// GET /group/loadMyGroup?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.LoadMyGroupRespond
func LoadMyGroupHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Group.LoadMyGroup(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// CheckGroupAddModeHandler 检查群聊加入方式
// GET /group/checkGroupAddMode?groupId=xxx
// 查询参数: request.CheckGroupAddModeRequest
// 响应: int8 (0=直接加入, 1=需要审核)
func CheckGroupAddModeHandler(c *gin.Context) {
	var req request.CheckGroupAddModeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	addMode, err := service.Svc.Group.CheckGroupAddMode(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, addMode)
}

// EnterGroupDirectlyHandler 直接加入群聊（无需审核）
// POST /group/enterGroupDirectly
// 请求体: request.EnterGroupDirectlyRequest
// 响应: nil
func EnterGroupDirectlyHandler(c *gin.Context) {
	var req request.EnterGroupDirectlyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.EnterGroupDirectly(req.GroupId, req.UserId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// LeaveGroupHandler 退出群聊
// POST /group/leaveGroup
// 请求体: request.LeaveGroupRequest
// 响应: nil
func LeaveGroupHandler(c *gin.Context) {
	var req request.LeaveGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.LeaveGroup(req.UserId, req.GroupId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// DismissGroupHandler 解散群聊（仅群主可操作）
// POST /group/dismissGroup
// 请求体: request.DismissGroupRequest
// 响应: nil
func DismissGroupHandler(c *gin.Context) {
	var req request.DismissGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.DismissGroup(req.OwnerId, req.GroupId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetGroupInfoHandler 获取群聊详细信息
// GET /group/getGroupInfo?groupId=xxx
// 查询参数: request.GetGroupInfoRequest
// 响应: respond.GetGroupInfoRespond
func GetGroupInfoHandler(c *gin.Context) {
	var req request.GetGroupInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Group.GetGroupInfo(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetGroupInfoListHandler 获取群聊列表（管理员功能）
// GET /group/getGroupInfoList?page=1&pageSize=10
// 查询参数: request.GetGroupListRequest
// 响应: respond.GetGroupListWrapper
func GetGroupInfoListHandler(c *gin.Context) {
	var req request.GetGroupListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Group.GetGroupInfoList(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// DeleteGroupsHandler 批量删除群聊（管理员功能）
// POST /group/deleteGroups
// 请求体: request.DeleteGroupsRequest
// 响应: nil
func DeleteGroupsHandler(c *gin.Context) {
	var req request.DeleteGroupsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.DeleteGroups(req.UuidList); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// SetGroupsStatusHandler 批量设置群聊状态（管理员功能）
// POST /group/setGroupsStatus
// 请求体: request.SetGroupsStatusRequest
// 响应: nil
func SetGroupsStatusHandler(c *gin.Context) {
	var req request.SetGroupsStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.SetGroupsStatus(req.UuidList, req.Status); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// UpdateGroupInfoHandler 更新群聊信息
// POST /group/updateGroupInfo
// 请求体: request.UpdateGroupInfoRequest
// 响应: nil
func UpdateGroupInfoHandler(c *gin.Context) {
	var req request.UpdateGroupInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.UpdateGroupInfo(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetGroupMemberListHandler 获取群成员列表
// GET /group/getGroupMemberList?groupId=xxx
// 查询参数: request.GetGroupMemberListRequest
// 响应: []respond.GetGroupMemberListRespond
func GetGroupMemberListHandler(c *gin.Context) {
	var req request.GetGroupMemberListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Group.GetGroupMemberList(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// RemoveGroupMembersHandler 移除群成员
// POST /group/removeGroupMembers
// 请求体: request.RemoveGroupMembersRequest
// 响应: nil
func RemoveGroupMembersHandler(c *gin.Context) {
	var req request.RemoveGroupMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.RemoveGroupMembers(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
