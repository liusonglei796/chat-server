package respond

// GetCurContactListInChatRoomRespond 获取当前在线联系人列表响应
// 使用位置:
//   - internal/service/chatroom/service.go: GetCurContactListInChatRoom
type GetCurContactListInChatRoomRespond struct {
	ContactId string `json:"contact_id"`
}
