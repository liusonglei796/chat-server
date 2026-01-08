package request

// ApplyRequest 申请添加联系人请求
// 使用位置:
//   - handler/contact_handler.go: ApplyContactHandler
type ApplyRequest struct {
	UserId    string `json:"user_id" binding:"required"`
	ContactId string `json:"contact_id" binding:"required"`
	Message   string `json:"message"`
}
