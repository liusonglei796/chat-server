package request

// ApplyGroupRequest 申请入群请求
// 使用位置:
//   - handler/contact_handler.go: ApplyGroupHandler
type ApplyGroupRequest struct {
	// UserId 申请人用户ID
	UserId string `json:"user_id" binding:"required"`
	// GroupId 被申请加入的群组ID
	GroupId string `json:"group_id" binding:"required"`
	// Message 申请附言
	Message string `json:"message"`
}
