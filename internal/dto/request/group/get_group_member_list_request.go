package group

// GetGroupMemberListRequest 获取群成员列表请求
// 使用位置: handler/group_handler.go: GetGroupMemberList
type GetGroupMemberListRequest struct {
	GroupId string `json:"group_id" form:"group_id" binding:"required"` // 群组UUID
}