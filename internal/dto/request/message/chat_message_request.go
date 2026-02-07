package message

// ChatMessageRequest 聊天消息请求 (WebSocket)
// 使用位置: handler/ws_handler.go: HandleWebSocketMessage
//
// 说明: 通过 WebSocket 发送聊天消息，支持文本、图片、文件、音视频等多种类型
type ChatMessageRequest struct {
	SessionId  string `json:"session_id"`                    // 会话ID
	Type       int8   `json:"type" binding:"required"`       // 消息类型
	Content    string `json:"content"`                       // 消息内容
	Url        string `json:"url"`                           // 文件URL
	SendId     string `json:"send_id" binding:"required"`    // 发送者ID
	SendName   string `json:"send_name"`                     // 发送者昵称
	SendAvatar string `json:"send_avatar"`                   // 发送者头像
	ReceiveId  string `json:"receive_id" binding:"required"` // 接收者ID（用户ID或群组ID）
	FileSize   string `json:"file_size"`                     // 文件大小
	FileType   string `json:"file_type"`                     // 文件类型
	FileName   string `json:"file_name"`                     // 文件名
	AVdata     string `json:"av_data"`                       // 音视频数据（JSON字符串）
}

// AVData 音视频消息数据
// 说明: 音视频通话的信号数据，包含消息ID和类型
type AVData struct {
	MessageId string `json:"messageId"` // 消息ID
	Type      string `json:"type"`      // 音视频类型（offer/answer/candidate等）
}
