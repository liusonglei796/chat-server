package request

// BlackGroupApplyRequest 拉黑入群申请请求
// 使用位置:
//   - handler/contact_handler.go: BlackGroupApplyHandler
type BlackGroupApplyRequest struct {
	// GroupId 群组ID
	GroupId string `json:"group_id" binding:"required"`
	// ApplicantId 申请人的用户ID
	ApplicantId string `json:"applicant_id" binding:"required"`
}
