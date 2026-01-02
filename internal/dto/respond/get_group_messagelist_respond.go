package respond

// GetGroupMessageListRespond 获取群聊消息记录响应
// 使用位置:
//   - internal/service/logic/message_service.go: GetGroupMessageList
//   - internal/service/chat/server.go: Start (群消息转发)
//   - internal/service/chat/kafka_server.go: Start (群消息转发)
type GetGroupMessageListRespond struct {
	SendId     string `json:"send_id"`
	SendName   string `json:"send_name"`
	SendAvatar string `json:"send_avatar"`
	ReceiveId  string `json:"receive_id"`
	Type       int8   `json:"type"`
	Content    string `json:"content"`
	Url        string `json:"url"`
	FileType   string `json:"file_type"`
	FileName   string `json:"file_name"`
	FileSize   string `json:"file_size"`
	CreatedAt  string `json:"created_at"` // 先用CreatedAt排序，后面考虑改成SentAt
}
