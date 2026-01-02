package request

// LeaveGroupRequest 退出群聊请求
// 使用位置:
//   - api/v1/group_info_controller.go: LeaveGroupHandler
type LeaveGroupRequest struct {
	UserId  string `json:"user_id" binding:"required"`
	GroupId string `json:"group_id" binding:"required"`
}
