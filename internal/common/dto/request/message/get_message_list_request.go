package message

// GetMessageListRequest 获取聊天记录请求（私聊 + 群聊通用）
// 私聊时 TargetId 为对方用户UUID，群聊时为群组UUID
// 使用位置: handler/message_handler.go
// 注意: 当前用户ID 从 JWT 上下文获取，防止 IDOR 攻击
//
// 支持两种分页方式：
// 1. 传统分页：使用 page 参数（已不推荐）
// 2. 游标分页：使用 cursor 参数（推荐，性能更好）
type GetMessageListRequest struct {
	TargetId string `form:"target_id" binding:"required"` // 目标ID（用户UUID 或 群组UUID）
	Page     int    `form:"page"`                         // 页码，默认为1（传统分页，已不推荐使用）
	PageSize int    `form:"page_size"`                    // 每页数量，默认为20
	Cursor   string `form:"cursor"`                       // 游标分页：上一页最后一条消息的时间戳（Unix时间戳字符串）
}
