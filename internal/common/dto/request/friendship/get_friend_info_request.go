package friendship

// GetFriendInfoRequest 获取好友信息请求
type GetFriendInfoRequest struct {
	FriendId string `json:"friend_id" form:"friend_id" binding:"required"` // 好友用户UUID
}
