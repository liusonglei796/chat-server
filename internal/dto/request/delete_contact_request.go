package request

// DeleteContactRequest 删除联系人请求
// 使用位置:
//   - handler/contact_handler.go: DeleteContactHandler
//
// 注意: UserId 从 JWT 上下文获取，不从请求体读取
type DeleteContactRequest struct {
	ContactId string `json:"contact_id" binding:"required"`
}
