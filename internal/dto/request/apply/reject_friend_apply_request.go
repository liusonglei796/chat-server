package apply

// RejectFriendApplyRequest 拒绝好友申请请求
// 使用位置: handler/apply_handler.go: RejectFriendApplyHandler
// 注意: UserId (当前用户) 从 JWT 上下文获取
type RejectFriendApplyRequest struct {
	ApplicantId string `json:"applicant_id" binding:"required"` // 申请人的用户ID
}
