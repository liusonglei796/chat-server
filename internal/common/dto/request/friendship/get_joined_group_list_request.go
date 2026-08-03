package friendship

// GetJoinedGroupListRequest 获取已加入的群组列表请求
type GetJoinedGroupListRequest struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}
