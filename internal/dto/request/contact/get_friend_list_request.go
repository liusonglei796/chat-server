package contact

// GetFriendListRequest 获取好友列表请求
// 使用位置: internal/handler/contact_handler.go: GetUserList
type GetFriendListRequest struct {
	// Page 页码，从1开始（可选，默认为1）
	Page int `form:"page" binding:"omitempty,min=1"`
	// PageSize 每页数量（可选，默认为20，最大100）
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}