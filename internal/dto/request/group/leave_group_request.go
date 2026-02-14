package group

// LeaveGroupRequest 退出群聊请求
// 使用位置:
//   - handler/group_handler.go: LeaveGroupHandler
//
// 注意: UserId 从 JWT 上下文获取，不从请求体读取
type LeaveGroupRequest struct {
	GroupId string `json:"group_id" binding:"required"`
}
