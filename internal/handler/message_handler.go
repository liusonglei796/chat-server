// Package handler 提供 HTTP 请求处理器
// 本文件处理消息和文件上传相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// GetMessageListHandler 获取两个用户之间的聊天记录
// GET /message/getMessageList?userOneId=xxx&userTwoId=xxx
// 查询参数: request.GetMessageListRequest
// 响应: []respond.GetMessageListRespond
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

// GetGroupMessageListHandler 获取群聊消息记录
// GET /message/getGroupMessageList?groupId=xxx
// 查询参数: request.GetGroupMessageListRequest
// 响应: []respond.GetGroupMessageListRespond
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

// UploadAvatarHandler 上传用户头像
// POST /message/uploadAvatar
// 请求体: multipart/form-data
// 响应: string (新头像文件名)
// 限制: 仅支持 image/jpeg, image/png, image/gif
func UploadAvatarHandler(c *gin.Context) {
	path, err := service.Svc.Message.UploadAvatar(c)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, path)
}

// UploadFileHandler 上传聊天文件
// POST /message/uploadFile
// 请求体: multipart/form-data
// 响应: []string (上传成功的文件名列表)
func UploadFileHandler(c *gin.Context) {
	paths, err := service.Svc.Message.UploadFile(c)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, paths)
}
