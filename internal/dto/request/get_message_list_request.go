package request

// GetMessageListRequest 获取聊天记录请求
// 使用位置:
//   - api/v1/message_controller.go: GetMessageListHandler
type GetMessageListRequest struct {
	UserOneId string `json:"user_one_id" form:"user_one_id" binding:"required"`
	UserTwoId string `json:"user_two_id" form:"user_two_id" binding:"required"`
}
