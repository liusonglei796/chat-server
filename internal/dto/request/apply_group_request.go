package request

// ApplyGroupRequest 申请入群请求
// 使用位置:
//   - handler/contact_handler.go: ApplyGroupHandler
//
// 注意: UserId (申请人) 通过 JWT 获取，作为参数传递给 Service
type ApplyGroupRequest struct {
	// GroupId 被申请加入的群组ID
	GroupId string `json:"group_id" binding:"required"`
	// Message 申请附言
	Message string `json:"message"`
}
