package apply

// RejectGroupApplyRequest 拒绝入群申请请求
// 使用位置: handler/contact_handler.go: RejectGroupApplyHandler
type RejectGroupApplyRequest struct {
	GroupId     string `json:"group_id" binding:"required"`     // 群组ID
	ApplicantId string `json:"applicant_id" binding:"required"` // 申请人的用户ID
}
