package handler

import (
	"github.com/gin-gonic/gin"

	adminreq "kama_chat_server/internal/dto/request/admin"
	"kama_chat_server/internal/dto/request/friendship"
	"kama_chat_server/internal/service"
)

// AdminHandler 后台管理请求处理器
type AdminHandler struct {
	userAdminSvc  service.UserAdminService
	groupAdminSvc service.GroupAdminService
}

// NewAdminHandler 创建后台管理处理器实例
func NewAdminHandler(userAdminSvc service.UserAdminService, groupAdminSvc service.GroupAdminService) *AdminHandler {
	return &AdminHandler{
		userAdminSvc:  userAdminSvc,
		groupAdminSvc: groupAdminSvc,
	}
}

// ==================== 用户管理 ====================

// GetUserListPaged 分页获取用户列表
// GET /admin/user/list
// 响应: respond.PagedUserListRespond
func (h *AdminHandler) GetUserListPaged(c *gin.Context) {
	var req adminreq.GetUserListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleError(c, err)
		return
	}

	data, err := h.userAdminSvc.GetUserListPaged(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// BatchUpdateUserStatus 批量更新用户状态
// POST /admin/user/batchStatus
// 请求: adminreq.BatchUpdateUserStatusRequest
func (h *AdminHandler) BatchUpdateUserStatus(c *gin.Context) {
	var req adminreq.BatchUpdateUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleError(c, err)
		return
	}

	if err := h.userAdminSvc.BatchUpdateUserStatus(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// SetAdmin 设置管理员权限
// POST /admin/user/setAdmin
// 请求: adminreq.SetUserAdminRequest
func (h *AdminHandler) SetAdmin(c *gin.Context) {
	var req adminreq.SetUserAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleError(c, err)
		return
	}

	if err := h.userAdminSvc.SetAdmin(req.UserUUIDs, req.IsAdmin); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// ==================== 群组管理 ====================

// GetGroupInfoList 分页获取群组列表
// GET /admin/group/list
// 响应: respond.GetGroupListWrapper
func (h *AdminHandler) GetGroupInfoList(c *gin.Context) {
	var req adminreq.GetGroupInfoListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleError(c, err)
		return
	}

	data, err := h.groupAdminSvc.GetGroupInfoList(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// DeleteGroups 批量删除群组
// POST /admin/group/delete
// 请求: contact.BatchDeleteRequest
func (h *AdminHandler) DeleteGroups(c *gin.Context) {
	var req friendship.BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleError(c, err)
		return
	}

	if err := h.groupAdminSvc.DeleteGroups(req.UuidList); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// SetGroupsStatus 批量设置群组状态
// POST /admin/group/setStatus
// 请求: adminreq.BatchUpdateGroupStatusRequest
func (h *AdminHandler) SetGroupsStatus(c *gin.Context) {
	var req adminreq.BatchUpdateGroupStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleError(c, err)
		return
	}

	// 将 action 转换为 status
	var status int8
	switch req.Action {
	case "enable":
		status = 1
	case "disable":
		status = 2
	case "delete":
		status = 0
	}

	if err := h.groupAdminSvc.SetGroupsStatus(req.GroupUUIDs, status); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
