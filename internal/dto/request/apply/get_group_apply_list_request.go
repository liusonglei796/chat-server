package apply

// GetGroupApplyListRequest 获取入群申请列表请求
// 使用位置: internal/handler/apply_handler.go: GetGroupApplyList
type GetGroupApplyListRequest struct {
	GroupId string `json:"group_id" form:"group_id" binding:"required"`
}
