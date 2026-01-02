package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterContactRoutes 注册联系人相关路由
func RegisterContactRoutes(r *gin.Engine) {
	r.GET("/contact/getUserList", handler.GetUserListHandler)
	r.GET("/contact/loadMyJoinedGroup", handler.LoadMyJoinedGroupHandler)
	r.GET("/contact/getContactInfo", handler.GetContactInfoHandler)
	r.POST("/contact/deleteContact", handler.DeleteContactHandler)
	r.POST("/contact/applyContact", handler.ApplyContactHandler)
	r.GET("/contact/getNewContactList", handler.GetNewContactListHandler)
	r.POST("/contact/passContactApply", handler.PassContactApplyHandler)
	r.POST("/contact/refuseContactApply", handler.RefuseContactApplyHandler)
	r.POST("/contact/blackContact", handler.BlackContactHandler)
	r.POST("/contact/cancelBlackContact", handler.CancelBlackContactHandler)
	r.GET("/contact/getAddGroupList", handler.GetAddGroupListHandler)
	r.POST("/contact/blackApply", handler.BlackApplyHandler)
}
