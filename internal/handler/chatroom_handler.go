package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// GetCurContactListInChatRoom 获取当前聊天室联系人列表
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
