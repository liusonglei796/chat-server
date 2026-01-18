package request

// GetUserListPagedRequest 分页获取用户列表请求（管理员）
// 使用位置:
//   - internal/handler/user_handler.go: GetUserListPaged
type GetUserListPagedRequest struct {
	Page     int    `form:"page" binding:"required,min=1"`
	PageSize int    `form:"pageSize" binding:"required,min=1,max=100"`
	Keyword  string `form:"keyword"` // 可选：搜索昵称/手机号
	Status   *int8  `form:"status"`  // 可选：按状态筛选
}
