package request

// ApplyFriendRequest 申请添加好友请求
// 使用位置:
//   - handler/contact_handler.go: ApplyFriendHandler
type ApplyFriendRequest struct {
	// UserId 申请人用户ID
	UserId string `json:"user_id" binding:"required"`
	// FriendId 被申请添加的好友用户ID
	FriendId string `json:"friend_id" binding:"required"`
	// Message 申请附言
	Message string `json:"message"`
}
