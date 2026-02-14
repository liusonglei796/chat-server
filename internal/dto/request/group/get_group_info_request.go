package group

// GetGroupInfoRequest 获取群组信息请求
// 使用位置: handler/group_handler.go: GetGroupInfo
type GetGroupInfoRequest struct {
	GroupId string `json:"group_id" form:"group_id" binding:"required"` // 群组UUID
}
