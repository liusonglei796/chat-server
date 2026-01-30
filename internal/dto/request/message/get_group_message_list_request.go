package message

// GetGroupMessageListRequest 获取群聊消息记录请求
// 使用位置: handler/message_handler.go: GetGroupMessageList
type GetGroupMessageListRequest struct {
	GroupId string `json:"group_id" form:"group_id" binding:"required"` // 群组UUID
}