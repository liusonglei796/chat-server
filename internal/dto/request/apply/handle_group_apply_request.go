package apply

// HandleGroupApplyRequest 处理入群申请请求（通过/拒绝/拉黑）
// 使用位置: handler/apply_handler.go
//
// 注意: OperatorId (当前用户) 从 JWT 上下文获取，不从请求体读取
type HandleGroupApplyRequest struct {
	// GroupId 群组ID
	GroupId string `json:"group_id" binding:"required"`
	// ApplicantId 申请人的用户ID
	ApplicantId string `json:"applicant_id" binding:"required"`
}
