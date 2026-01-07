package request

// BlackFriendApplyRequest 拉黑好友申请请求
// 使用位置:
//   - handler/contact_handler.go: BlackFriendApplyHandler
type BlackFriendApplyRequest struct {
	// UserId 当前用户ID（被申请添加的好友）
	UserId string `json:"user_id" binding:"required"`
	// ApplicantId 申请人的用户ID
	ApplicantId string `json:"applicant_id" binding:"required"`
}
