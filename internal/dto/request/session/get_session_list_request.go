package session

// GetSessionListRequest 获取会话列表请求（分页）
// 使用位置: handler/session_handler.go: GetUserSessionList, GetGroupSessionList
type GetSessionListRequest struct {
	Page     int `form:"page"`      // 页码，默认为1
	PageSize int `form:"page_size"` // 每页数量，默认为20
}
