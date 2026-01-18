package request

// BatchUpdateUserStatusRequest 批量更新用户状态请求（管理员）
// Action 可选值: enable(启用), disable(禁用), delete(删除)
// 使用位置:
//   - internal/handler/user_handler.go: BatchUpdateUserStatus
type BatchUpdateUserStatusRequest struct {
	UuidList []string `json:"uuid_list" binding:"required,min=1"`
	Action   string   `json:"action" binding:"required,oneof=enable disable delete"`
}
