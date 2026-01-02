package request

// GetCurContactListInChatRoomRequest 获取聊天室在线联系人请求
type GetCurContactListInChatRoomRequest struct {
	UserId    string `json:"user_id" form:"user_id" binding:"required"`
	ContactId string `json:"contact_id" form:"contact_id" binding:"required"`
}
