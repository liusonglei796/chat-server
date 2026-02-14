package apply

// HandleFriendApplyRequest 处理好友申请请求（通过/拒绝/拉黑）
// 使用位置: handler/apply_handler.go
//
// 注意: UserId (当前用户) 从 JWT 上下文获取，不从请求体读取
type HandleFriendApplyRequest struct {
	// ApplicantId 申请人的用户ID
	ApplicantId string `json:"applicant_id" binding:"required"`
}
