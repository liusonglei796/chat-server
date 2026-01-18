package request

// BlackContactRequest 拉黑/取消拉黑联系人请求
// 使用位置:
//   - handler/contact_handler.go: BlackContactHandler, CancelBlackContactHandler
//
// 注意: UserId 从 JWT 上下文获取，不从请求体读取
type BlackContactRequest struct {
	ContactId string `json:"contact_id" binding:"required"`
}
