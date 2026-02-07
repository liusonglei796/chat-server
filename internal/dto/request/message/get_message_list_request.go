package message

// GetMessageListRequest 获取聊天记录请求
// 使用位置: api/v1/message_controller.go: GetMessageListHandler
// 注意: UserOneId 从 JWT 上下文获取，防止 IDOR 攻击
type GetMessageListRequest struct {
	PartnerId string `form:"partner_id" binding:"required"` // 对方用户ID（对方用户UUID）
	Page      int    `form:"page"`                          // 页码，默认为1
	PageSize  int    `form:"page_size"`                     // 每页数量，默认为20
}
