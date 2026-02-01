package message

// GetGroupMessageListRequest 获取群聊消息记录请求
// 使用位置: handler/message_handler.go: GetGroupMessageList
type GetGroupMessageListRequest struct {
	GroupId  string `json:"group_id" form:"group_id" binding:"required"` // 群组UUID
	Page     int    `form:"page"`                                        // 页码，默认为1
	PageSize int    `form:"page_size"`                                   // 每页数量，默认为20
}
