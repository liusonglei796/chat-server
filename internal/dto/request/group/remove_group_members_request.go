package group

// RemoveGroupMembersRequest 移除群成员请求
// 使用位置: handler/group_handler.go: RemoveGroupMembers
//
// 说明: 仅群主或管理员可调用，支持批量移除群成员
type RemoveGroupMembersRequest struct {
	GroupId  string   `json:"group_id" binding:"required"`        // 群组ID
	UuidList []string `json:"uuid_list" binding:"required,min=1"` // 要移除的成员ID列表
}
