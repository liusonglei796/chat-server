package request

// SetAdminRequest 设置管理员请求
type SetAdminRequest struct {
	UuidList []string `json:"uuid_list" binding:"required"`
	IsAdmin  int8     `json:"is_admin"`
}
