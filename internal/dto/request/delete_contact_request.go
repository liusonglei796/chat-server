package request

// DeleteContactRequest 删除联系人请求
// 使用位置:
//   - handler/contact_handler.go: DeleteContactHandler
type DeleteContactRequest struct {
	UserId    string `json:"user_id" binding:"required"`
	ContactId string `json:"contact_id" binding:"required"`
}
