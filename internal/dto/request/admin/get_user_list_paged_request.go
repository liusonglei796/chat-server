package admin

// GetUserListRequest 分页获取用户列表请求（管理员）
// 使用位置:
//   - handler/admin_handler.go: GetUserListPaged
//
// 说明: 仅管理员可调用，返回所有用户的分页列表，支持关键词搜索和状态筛选
type GetUserListRequest struct {
	Page     int    `form:"page" binding:"required,min=1"`           // 页码，从1开始
	PageSize int    `form:"page_size" binding:"required,min=1,max=100"` // 每页数量，最大100
	Keyword  string `form:"keyword"`                                 // 可选：搜索昵称/手机号
	Status   *int8  `form:"status"`                                  // 可选：按状态筛选
}
