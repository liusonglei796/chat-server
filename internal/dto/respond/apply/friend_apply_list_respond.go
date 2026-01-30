package apply

// FriendApplyListRespond 好友申请列表响应
// 使用位置:
//   - internal/service/apply/service.go: GetFriendApplyList
type FriendApplyListRespond struct {
	ApplicantId     string `json:"applicant_id"`
	ApplicantName   string `json:"applicant_name"`
	ApplicantAvatar string `json:"applicant_avatar"`
	Message         string `json:"message"`
}
