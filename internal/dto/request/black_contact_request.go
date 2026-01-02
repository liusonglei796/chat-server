package request

// BlackContactRequest 拉黑/取消拉黑联系人请求
// 使用位置:
//   - handler/contact_handler.go: BlackContactHandler, CancelBlackContactHandler
type BlackContactRequest struct {
	UserId    string `json:"user_id" binding:"required"`
	ContactId string `json:"contact_id" binding:"required"`
}
