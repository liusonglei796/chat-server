package friendship

// GetFriendListRequest 获取好友列表请求
type GetFriendListRequest struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}
