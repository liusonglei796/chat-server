package contact

// BlockContactRequest 拉黑/取消拉黑联系人请求
// 使用位置: handler/contact_handler.go: BlockContactHandler, UnblockContactHandler
// 注意: UserId 从 JWT 上下文获取，不从请求体读取
type BlockContactRequest struct {
	ContactId string `json:"contact_id" binding:"required"` // 被拉黑的好友用户ID
}
