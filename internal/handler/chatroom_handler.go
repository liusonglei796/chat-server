// Package handler 提供 HTTP 请求处理器
// 本文件处理聊天室相关的 API 请求
package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// GetCurContactListInChatRoomHandler 获取当前聊天室中的联系人列表
// GET /chatroom/getCurContactListInChatRoom?userId=xxx&contactId=xxx
// 查询参数: request.GetCurContactListInChatRoomRequest
// 响应: []respond.GetCurContactListInChatRoomRespond
func GetCurContactListInChatRoomHandler(c *gin.Context) {
	var req request.GetCurContactListInChatRoomRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.ChatRoom.GetCurContactListInChatRoom(req.UserId, req.ContactId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
