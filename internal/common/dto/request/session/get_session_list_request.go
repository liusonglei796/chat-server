package session

// GetSessionListRequest 获取会话列表请求
// 使用位置: handler/session_handler.go: GetUserSessionList, GetGroupSessionList
//
// 支持两种分页方式：
// 1. 传统分页：使用 page 参数（已不推荐）
// 2. 游标分页：使用 cursor 参数（推荐，性能更好）
type GetSessionListRequest struct {
	Page     int    `form:"page"`      // 页码，默认为1（传统分页，已不推荐使用）
	PageSize int    `form:"page_size"` // 每页数量，默认为20
	Cursor   string `form:"cursor"`    // 游标分页：上一页最后一条会话的时间戳（Unix时间戳字符串）
}
