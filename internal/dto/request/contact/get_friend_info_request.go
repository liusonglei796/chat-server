package contact

// GetFriendInfoRequest 获取好友信息请求
// 使用位置: handler/contact_handler.go: GetFriendInfo
type GetFriendInfoRequest struct {
	FriendId string `json:"friend_id" form:"friend_id" binding:"required"` // 好友用户UUID
}