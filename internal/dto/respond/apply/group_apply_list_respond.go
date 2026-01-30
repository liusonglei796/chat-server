package apply

// GroupApplyListRespond 加群申请列表响应
// 使用位置:
//   - internal/service/apply/service.go: GetGroupApplyList
type GroupApplyListRespond struct {
	ApplicantId     string `json:"applicant_id"`
	ApplicantName   string `json:"applicant_name"`
	ApplicantAvatar string `json:"applicant_avatar"`
	Message         string `json:"message"`
}
