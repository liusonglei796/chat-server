package admin

// GetGroupInfoListRequest 分页获取群组列表请求（管理员）
// 使用位置:
//   - handler/admin_handler.go: GetGroupInfoList
//
// 说明: 仅管理员可调用，返回所有群组的分页列表
type GetGroupInfoListRequest struct {
	Page     int `form:"page" binding:"required,min=1"`           // 页码，从1开始
	PageSize int `form:"page_size" binding:"required,min=1,max=100"` // 每页数量，最大100
}