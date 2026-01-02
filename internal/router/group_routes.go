package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterGroupRoutes 注册群组相关路由
func RegisterGroupRoutes(r *gin.Engine) {
	r.POST("/group/createGroup", handler.CreateGroupHandler)
	r.GET("/group/loadMyGroup", handler.LoadMyGroupHandler)
	r.GET("/group/checkGroupAddMode", handler.CheckGroupAddModeHandler)
	r.POST("/group/enterGroupDirectly", handler.EnterGroupDirectlyHandler)
	r.POST("/group/leaveGroup", handler.LeaveGroupHandler)
	r.POST("/group/dismissGroup", handler.DismissGroupHandler)
	r.GET("/group/getGroupInfo", handler.GetGroupInfoHandler)
	r.GET("/group/getGroupInfoList", handler.GetGroupInfoListHandler)
	r.POST("/group/deleteGroups", handler.DeleteGroupsHandler)
	r.POST("/group/setGroupsStatus", handler.SetGroupsStatusHandler)
	r.POST("/group/updateGroupInfo", handler.UpdateGroupInfoHandler)
	r.GET("/group/getGroupMemberList", handler.GetGroupMemberListHandler)
	r.POST("/group/removeGroupMembers", handler.RemoveGroupMembersHandler)
}
