package apply

// GetFriendApplyListRequest 获取好友申请列表请求
// 使用位置: internal/handler/apply_handler.go: GetFriendApplyList
//
// 为什么需要 DTO:
//   - 好友申请列表 (GET /apply/friendList): 原本无查询参数，现在支持分页，需要DTO
//   - 群组申请列表 (GET /apply/groupList): 需要groupId查询参数和分页参数，必须有DTO
type GetFriendApplyListRequest struct {
	// Page 页码，从1开始（可选，默认为1）
	Page int `form:"page" binding:"omitempty,min=1"`
	// PageSize 每页数量（可选，默认为20，最大100）
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}
