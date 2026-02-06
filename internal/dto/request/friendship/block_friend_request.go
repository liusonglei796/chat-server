package friendship

// BlockFriendRequest 拉黑/取消拉黑好友请求
// 使用位置: handler/friendship_handler.go: BlockFriend, UnblockFriend
// 注意: UserId 从 JWT 上下文获取，不从请求体读取
type BlockFriendRequest struct {
	FriendId string `json:"friend_id" binding:"required"` // 被拉黑的好友用户ID
}
