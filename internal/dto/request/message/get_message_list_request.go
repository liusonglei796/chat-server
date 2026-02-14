package message

// GetMessageListRequest 获取聊天记录请求（私聊 + 群聊通用）
// 私聊时 TargetId 为对方用户UUID，群聊时为群组UUID
// 使用位置: handler/message_handler.go
// 注意: 当前用户ID 从 JWT 上下文获取，防止 IDOR 攻击
type GetMessageListRequest struct {
	TargetId string `form:"target_id" binding:"required"` // 目标ID（用户UUID 或 群组UUID）
	Page     int    `form:"page"`                         // 页码，默认为1
	PageSize int    `form:"page_size"`                    // 每页数量，默认为20
}
