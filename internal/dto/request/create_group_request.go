package request

// CreateGroupRequest 创建群聊请求
// 使用位置:
//   - api/v1/group_info_controller.go: CreateGroupHandler
//   - internal/service/logic/group_info_service.go: CreateGroup
type CreateGroupRequest struct {
	OwnerId string `json:"owner_id" binding:"required"`
	Name    string `json:"name" binding:"required"`
	Notice  string `json:"notice"`
	AddMode int8   `json:"add_mode"`
	Avatar  string `json:"avatar"`
}
