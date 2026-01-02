package respond

// GetGroupMemberListRespond 获取群成员列表响应
// 使用位置:
//   - internal/service/logic/group_info_service.go: GetGroupMemberList
type GetGroupMemberListRespond struct {
	UserId   string `json:"user_id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}
