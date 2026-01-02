package respond

// GetUserListRespond 获取用户列表响应 (管理员)
// 使用位置:
//   - internal/service/logic/user_info_service.go: GetUserInfoList
type GetUserListRespond struct {
	Uuid      string `json:"uuid"`
	Nickname  string `json:"nickname"`
	Telephone string `json:"telephone"`
	Status    int8   `json:"status"`
	IsAdmin   int8   `json:"is_admin"`
	IsDeleted bool   `json:"is_deleted"`
}
