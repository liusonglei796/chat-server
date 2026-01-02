package handler

import (
	"log"

	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// GetUserList 获取联系人列表
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

// LoadMyJoinedGroup 获取我加入的群聊
func LoadMyJoinedGroupHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.LoadMyJoinedGroup(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetContactInfo 获取联系人信息
func GetContactInfoHandler(c *gin.Context) {
	var req request.GetContactInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	log.Println(req)
	data, err := service.Svc.Contact.GetContactInfo(req.ContactId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// DeleteContact 删除联系人
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

// ApplyContact 申请添加联系人
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

// GetNewContactList 获取新的联系人申请列表
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

// PassContactApply 通过联系人申请
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

// RefuseContactApply 拒绝联系人申请
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

// BlackContact 拉黑联系人
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

// CancelBlackContact 解除拉黑联系人
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

// GetAddGroupList 获取新的群聊申请列表
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

// BlackApply 拉黑申请
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
