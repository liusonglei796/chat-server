package respond

// GetGroupListRespond 获取群聊列表响应 (管理员)
// 使用位置:
//   - internal/service/logic/group_info_service.go: GetGroupInfoList
type GetGroupListRespond struct {
	Uuid      string `json:"uuid"`
	Name      string `json:"name"`
	OwnerId   string `json:"owner_id"`
	Status    int8   `json:"status"`
	IsDeleted bool   `json:"is_deleted"`
}

type GetGroupListWrapper struct {
	List  []GetGroupListRespond `json:"list"`
	Total int64                 `json:"total"`
}
