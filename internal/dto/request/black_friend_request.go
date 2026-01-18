package request

// BlackFriendRequest 拉黑好友申请请求
// 使用位置:
//   - handler/apply_handler.go: BlackFriendApplyHandler
//
// 注意: UserId (当前用户) 从 JWT 上下文获取，不从请求体读取
type BlackFriendRequest struct {
	// ApplicantId 申请人的用户ID
	ApplicantId string `json:"applicant_id" binding:"required"`
}
