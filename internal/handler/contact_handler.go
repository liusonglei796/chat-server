// Package handler 提供 HTTP 请求处理器
// 本文件处理联系人相关的 API 请求
package handler

import (
	"log"

	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// GetUserListHandler 获取好友列表
// GET /contact/getUserList?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.MyUserListRespond
func GetUserListHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetUserList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// LoadMyJoinedGroupHandler 获取已加入的群组（排除自己创建的）
// GET /contact/loadMyJoinedGroup?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.LoadMyJoinedGroupRespond
func LoadMyJoinedGroupHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetJoinedGroupsExcludedOwn(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetContactInfoHandler 获取联系人详细信息
// GET /contact/getContactInfo?contactId=xxx
// 查询参数: request.GetContactInfoRequest
// 响应: respond.GetContactInfoRespond
func GetContactInfoHandler(c *gin.Context) {
	var req request.GetContactInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	log.Println(req) // 调试输出
	data, err := service.Svc.Contact.GetContactInfo(req.ContactId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// DeleteContactHandler 删除联系人
// POST /contact/deleteContact
// 请求体: request.DeleteContactRequest
// 响应: nil
func DeleteContactHandler(c *gin.Context) {
	var req request.DeleteContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.DeleteContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// ApplyContactHandler 申请添加联系人（好友/群组）
// POST /contact/applyContact
// 请求体: request.ApplyContactRequest
// 响应: nil
func ApplyContactHandler(c *gin.Context) {
	var req request.ApplyContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.ApplyContact(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetNewContactListHandler 获取待处理的联系人申请列表
// GET /contact/getNewContactList?userId=xxx
// 查询参数: request.OwnlistRequest
// 响应: []respond.NewContactListRespond
func GetNewContactListHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetNewContactList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// PassContactApplyHandler 通过联系人申请
// POST /contact/passContactApply
// 请求体: request.PassContactApplyRequest
// 响应: nil
func PassContactApplyHandler(c *gin.Context) {
	var req request.PassContactApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.PassContactApply(req.TargetId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// RefuseContactApplyHandler 拒绝联系人申请
// POST /contact/refuseContactApply
// 请求体: request.PassContactApplyRequest
// 响应: nil
func RefuseContactApplyHandler(c *gin.Context) {
	var req request.PassContactApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.RefuseContactApply(req.TargetId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// BlackContactHandler 拉黑联系人
// POST /contact/blackContact
// 请求体: request.BlackContactRequest
// 响应: nil
func BlackContactHandler(c *gin.Context) {
	var req request.BlackContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.BlackContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// CancelBlackContactHandler 取消拉黑联系人
// POST /contact/cancelBlackContact
// 请求体: request.BlackContactRequest
// 响应: nil
func CancelBlackContactHandler(c *gin.Context) {
	var req request.BlackContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.CancelBlackContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}

// GetAddGroupListHandler 获取入群申请列表
// GET /contact/getAddGroupList?groupId=xxx
// 查询参数: request.AddGroupListRequest
// 响应: []respond.AddGroupListRespond
func GetAddGroupListHandler(c *gin.Context) {
	var req request.AddGroupListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetAddGroupList(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// BlackApplyHandler 拉黑申请（不再接收该用户的申请）
// POST /contact/blackApply
// 请求体: request.BlackApplyRequest
// 响应: nil
func BlackApplyHandler(c *gin.Context) {
	var req request.BlackApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.BlackApply(req.TargetId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
