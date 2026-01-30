package admin

// BatchUpdateUserStatusRequest 批量更新用户状态请求
// 使用位置: handler/admin_handler.go: BatchUpdateUserStatus
//
// 说明: 仅管理员可调用，支持批量操作用户状态
//   - enable: 启用用户
//   - disable: 禁用用户
//   - delete: 删除用户
type BatchUpdateUserStatusRequest struct {
	UserUUIDs []string `json:"user_uuids" binding:"required,min=1"` // 用户ID列表
	Action    string   `json:"action" binding:"required,oneof=enable disable delete"` // 操作类型
}