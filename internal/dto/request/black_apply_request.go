package request

// BlackApplyRequest 拉黑申请请求
// 使用位置:
//   - handler/contact_handler.go: BlackApplyHandler
type BlackApplyRequest struct {
	// TargetId 被申请的目标（用户ID或群组ID）
	TargetId string `json:"target_id" binding:"required"`
	// ApplicantId 申请人的用户ID
	ApplicantId string `json:"applicant_id" binding:"required"`
}
