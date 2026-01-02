package request

// RemoveGroupMembersRequest 移除群成员请求
// 使用位置:
//   - api/v1/group_info_controller.go: RemoveGroupMembersHandler
//   - internal/service/logic/group_info_service.go: RemoveGroupMembers
type RemoveGroupMembersRequest struct {
	GroupId  string   `json:"group_id" binding:"required"`
	OwnerId  string   `json:"owner_id" binding:"required"`
	UuidList []string `json:"uuid_list" binding:"required,min=1"`
}
