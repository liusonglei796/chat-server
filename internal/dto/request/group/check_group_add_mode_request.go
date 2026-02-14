package group

// CheckGroupAddModeRequest 检查群组加群模式请求
// 使用位置: handler/group_handler.go: CheckGroupAddMode
type CheckGroupAddModeRequest struct {
	GroupId string `json:"group_id" form:"group_id" binding:"required"` // 群组UUID
}
