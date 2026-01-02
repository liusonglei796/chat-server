package request

// GetGroupMessageListRequest 获取群聊消息记录请求
// 使用位置:
//   - api/v1/message_controller.go: GetGroupMessageListHandler
type GetGroupMessageListRequest struct {
	GroupId string `json:"group_id" form:"group_id" binding:"required"`
}
