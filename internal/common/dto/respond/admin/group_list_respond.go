package admin

// GetGroupListRespond 获取群聊列表响应 (管理员)
type GetGroupListRespond struct {
	Uuid      string `json:"uuid"`
	Name      string `json:"name"`
	OwnerId   string `json:"owner_id"`
	Status    int8   `json:"status"`
	IsDeleted bool   `json:"is_deleted"`
}

// GetGroupListWrapper 群组列表包装响应
type GetGroupListWrapper struct {
	List  []GetGroupListRespond `json:"list"`
	Total int64                 `json:"total"`
}
