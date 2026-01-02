package request

// AbleUsersRequest 启用/禁用/删除/设置管理员用户请求
// 使用位置:
//   - api/v1/user_info_controller.go: AbleUsersHandler, DisableUsersHandler, DeleteUsersHandler, SetAdminHandler
type AbleUsersRequest struct {
	UuidList []string `json:"uuid_list" binding:"required,min=1"`
	IsAdmin  int8     `json:"is_admin"`
}
