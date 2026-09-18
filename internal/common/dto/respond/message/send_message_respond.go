package message

// SendMessageRespond 发送消息成功响应
type SendMessageRespond struct {
	MessageUuid string `json:"message_uuid"`
	CreatedAt   string `json:"created_at"`
}
