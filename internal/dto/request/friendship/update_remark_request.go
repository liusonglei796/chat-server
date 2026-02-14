package friendship

// UpdateRemarkRequest 更新好友备注请求
type UpdateRemarkRequest struct {
	// FriendId 好友 UUID
	FriendId string `json:"friend_id" binding:"required"`
	// Remark 备注名称，空字符串表示清除备注
	Remark string `json:"remark" binding:"max=50"`
}
