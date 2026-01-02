package respond

// AVMessageRespond 音视频通话消息响应
// 使用位置:
//   - internal/service/chat/server.go: Start (音视频消息转发)
//   - internal/service/chat/kafka_server.go: Start (音视频消息转发)
type AVMessageRespond struct {
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
	AVdata     string `json:"av_data"`
}
