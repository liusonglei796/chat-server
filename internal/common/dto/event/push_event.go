package event

// PushEvent 表示由业务服务（如MessageService）发出，
// 交由 ChatServer(WebSocket) 推送给指定客户端的事件。
type PushEvent struct {
	TargetUserId string `json:"target_user_id"` // 目标用户 UUID
	Payload      []byte `json:"payload"`        // 推送给客户端的 JSON 数据 (通常是 MessageBack 的 JSON)
	MessageUuid  string `json:"message_uuid"`   // 消息UUID（可选，用于标记已发送）
}
