package request

// PassContactApplyRequest 通过/拒绝联系人申请请求
// 使用位置:
//   - handler/contact_handler.go: PassContactApplyHandler, RefuseContactApplyHandler
type PassContactApplyRequest struct {
	// TargetId 被申请的目标（用户ID或群组ID）
	TargetId string `json:"target_id" binding:"required"`
	// ApplicantId 申请人的用户ID
	ApplicantId string `json:"applicant_id" binding:"required"`
}
