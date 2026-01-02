package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// CreateGroup 创建群聊
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

// LoadMyGroup 获取我创建的群聊
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

// CheckGroupAddMode 检查群聊加群方式
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

// EnterGroupDirectly 直接进群
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

// LeaveGroup 退群
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

// DismissGroup 解散群聊
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

// GetGroupInfo 获取群聊详情
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

// GetGroupInfoList 获取群聊列表 - 管理员
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

// DeleteGroups 删除列表中群聊 - 管理员
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

// SetGroupsStatus 设置群聊是否启用
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

// UpdateGroupInfo 更新群聊消息
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

// GetGroupMemberList 获取群聊成员列表
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

// RemoveGroupMembers 移除群聊成员
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
