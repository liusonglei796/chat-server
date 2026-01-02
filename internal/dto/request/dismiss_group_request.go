package request

// DismissGroupRequest 解散群聊请求
// 使用位置:
//   - api/v1/group_info_controller.go: DismissGroupHandler
type DismissGroupRequest struct {
	OwnerId string `json:"owner_id" binding:"required"`
	GroupId string `json:"group_id" binding:"required"`
}
