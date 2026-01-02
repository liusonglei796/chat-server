package request

// UpdateGroupInfoRequest 更新群聊信息请求
// 使用位置:
//   - api/v1/group_info_controller.go: UpdateGroupInfoHandler
//   - internal/service/logic/group_info_service.go: UpdateGroupInfo
type UpdateGroupInfoRequest struct {
	OwnerId string `json:"owner_id" binding:"required"`
	Uuid    string `json:"uuid" binding:"required"`
	Name    string `json:"name"`
	Avatar  string `json:"avatar"`
	AddMode int8   `json:"add_mode"`
	Notice  string `json:"notice"`
}
