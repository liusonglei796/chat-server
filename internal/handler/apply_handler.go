// Package handler 提供 HTTP 请求处理器
// 本文件处理申请相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request/apply"
	applysvc "kama_chat_server/internal/service/apply"
	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
)

// ApplyHandler 申请请求处理器
type ApplyHandler struct {
	applySvc *applysvc.ApplyService
}

// NewApplyHandler 创建申请处理器实例
func NewApplyHandler(applySvc *applysvc.ApplyService) *ApplyHandler {
	return &ApplyHandler{applySvc: applySvc}
}

// ApplyFriend 申请添加好友
// POST /apply/friend
// 请求体: apply.ApplyFriendRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，防止IDOR攻击
func (h *ApplyHandler) ApplyFriend(c *gin.Context) {
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
	if err := h.applySvc.ApplyFriend(userId.(string), req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// ApplyGroup 申请加入群组
// POST /apply/group
// 请求体: apply.ApplyGroupRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，防止IDOR攻击
func (h *ApplyHandler) ApplyGroup(c *gin.Context) {
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
	if err := h.applySvc.ApplyGroup(userId.(string), req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetFriendApplyList 获取待处理的好友申请列表
// GET /apply/friendList?page=1&page_size=20
// 从JWT上下文获取当前用户ID
// 响应: respond.PagedFriendApplyListRespond
func (h *ApplyHandler) GetFriendApplyList(c *gin.Context) {
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

	data, err := h.applySvc.GetFriendApplyList(userId.(string), req.Page, req.PageSize)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetGroupApplyList 获取入群申请列表
// GET /apply/groupList?groupId=xxx&page=1&page_size=20
// 查询参数: apply.GetGroupApplyListRequest
// 响应: respond.PagedGroupApplyListRespond
func (h *ApplyHandler) GetGroupApplyList(c *gin.Context) {
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
	data, err := h.applySvc.GetGroupApplyList(userId.(string), req.GroupId, req.Page, req.PageSize)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// PassFriendApply 通过好友申请
// POST /apply/passFriend
// 请求体: apply.HandleFriendApplyRequest
// 响应: nil
func (h *ApplyHandler) PassFriendApply(c *gin.Context) {
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
	if err := h.applySvc.PassFriendApply(userId.(string), req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// PassGroupApply 通过入群申请
// POST /apply/passGroup
// 请求体: apply.HandleGroupApplyRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，Service层校验审批权限
func (h *ApplyHandler) PassGroupApply(c *gin.Context) {
	operatorId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.HandleGroupApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.applySvc.PassGroupApply(operatorId.(string), req.GroupId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// RefuseFriendApply 拒绝好友申请
// POST /apply/refuseFriend
// 请求体: apply.HandleFriendApplyRequest
// 响应: nil
func (h *ApplyHandler) RefuseFriendApply(c *gin.Context) {
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

	if err := h.applySvc.RefuseFriendApply(userId.(string), req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// RefuseGroupApply 拒绝入群申请
// POST /apply/refuseGroup
// 请求体: apply.HandleGroupApplyRequest
// 响应: nil
// 安全: 从JWT上下文获取当前用户ID，Service层校验审批权限
func (h *ApplyHandler) RefuseGroupApply(c *gin.Context) {
	operatorId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}

	var req apply.HandleGroupApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.applySvc.RefuseGroupApply(operatorId.(string), req.GroupId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackFriendApply 拉黑好友申请
// POST /apply/blackFriend
// 请求体: apply.HandleFriendApplyRequest
// 响应: nil
func (h *ApplyHandler) BlackFriendApply(c *gin.Context) {
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
	if err := h.applySvc.BlackFriendApply(userId.(string), req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackGroupApply 拉黑入群申请
// POST /apply/blackGroup
// 请求体: apply.HandleGroupApplyRequest
// 响应: nil
func (h *ApplyHandler) BlackGroupApply(c *gin.Context) {
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
	if err := h.applySvc.BlackGroupApply(userId.(string), req.GroupId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
