package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// GetMessageList 获取聊天记录
func GetMessageListHandler(c *gin.Context) {
	var req request.GetMessageListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Message.GetMessageList(req.UserOneId, req.UserTwoId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetGroupMessageList 获取群聊消息记录
func GetGroupMessageListHandler(c *gin.Context) {
	var req request.GetGroupMessageListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Message.GetGroupMessageList(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// UploadAvatar 上传头像
func UploadAvatarHandler(c *gin.Context) {
	path, err := service.Svc.Message.UploadAvatar(c)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, path)
}

// UploadFile 上传文件
func UploadFileHandler(c *gin.Context) {
	paths, err := service.Svc.Message.UploadFile(c)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, paths)
}
