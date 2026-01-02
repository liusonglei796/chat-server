package request

// ChatMessageRequest 聊天消息请求 (WebSocket)
// 使用位置:
//   - internal/service/chat/client.go: Read
//   - internal/service/chat/server.go: Start
//   - internal/service/chat/kafka_server.go: Start
type ChatMessageRequest struct {
	SessionId  string `json:"session_id"`
	Type       int8   `json:"type" binding:"required"`
	Content    string `json:"content"`
	Url        string `json:"url"`
	SendId     string `json:"send_id" binding:"required"`
	SendName   string `json:"send_name"`
	SendAvatar string `json:"send_avatar"`
	ReceiveId  string `json:"receive_id" binding:"required"`
	FileSize   string `json:"file_size"`
	FileType   string `json:"file_type"`
	FileName   string `json:"file_name"`
	AVdata     string `json:"av_data"`
}
