package request

// RemoveGroupMembersRequest 移除群成员请求
// 使用位置:
//   - handler/group_handler.go: RemoveGroupMembersHandler
//
// 注意: 操作者ID 通过 JWT 获取，Service层校验权限
type RemoveGroupMembersRequest struct {
	GroupId  string   `json:"group_id" binding:"required"`
	UuidList []string `json:"uuid_list" binding:"required"`
}
