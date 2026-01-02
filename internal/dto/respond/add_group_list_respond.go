package respond

// AddGroupListRespond 加群申请列表响应
// 使用位置:
//   - internal/service/contact/service.go: GetAddGroupList
type AddGroupListRespond struct {
	ApplicantId   string `json:"applicant_id"`
	ContactName   string `json:"contact_name"`
	ContactAvatar string `json:"contact_avatar"`
	Message       string `json:"message"`
}
