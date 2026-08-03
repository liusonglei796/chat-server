package admin

// GetUserListRespond 获取用户列表响应 (管理员)
type GetUserListRespond struct {
	Uuid      string `json:"uuid"`
	Nickname  string `json:"nickname"`
	Telephone string `json:"telephone"`
	Status    int8   `json:"status"`
	IsAdmin   int8   `json:"is_admin"`
	IsDeleted bool   `json:"is_deleted"`
}

// PagedUserListRespond 分页用户列表响应（管理员）
type PagedUserListRespond struct {
	Total int64                `json:"total"`
	List  []GetUserListRespond `json:"list"`
}
