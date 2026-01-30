package admin

// SetUserAdminRequest 设置用户管理员权限请求
// 使用位置: handler/admin_handler.go: SetAdmin
//
// 说明: 仅管理员可调用，支持批量设置或取消用户的管理员权限
//   - is_admin=0: 取消管理员权限
//   - is_admin=1: 设置为管理员
type SetUserAdminRequest struct {
	UserUUIDs []string `json:"user_uuids" binding:"required,min=1"` // 用户ID列表
	IsAdmin   int8     `json:"is_admin" binding:"required,oneof=0 1"` // 是否为管理员
}