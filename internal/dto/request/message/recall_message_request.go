package message

// RecallMessageRequest 消息撤回请求
type RecallMessageRequest struct {
	// MessageUuid 消息唯一标识
	MessageUuid string `json:"message_uuid" binding:"required"`
}
