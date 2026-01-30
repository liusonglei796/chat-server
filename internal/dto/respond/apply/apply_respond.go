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

// GroupApplyListRespond 加群申请列表响应
// 使用位置:
//   - internal/service/apply/service.go: GetGroupApplyList
type GroupApplyListRespond struct {
	ApplicantId     string `json:"applicant_id"`
	ApplicantName   string `json:"applicant_name"`
	ApplicantAvatar string `json:"applicant_avatar"`
	Message         string `json:"message"`
}

// PagedGroupApplyListRespond 分页入群申请列表响应
type PagedGroupApplyListRespond struct {
	Total int64                      `json:"total"` // 总数量
	List  []GroupApplyListRespond    `json:"list"`  // 申请列表
}

// PagedFriendApplyListRespond 分页好友申请列表响应
type PagedFriendApplyListRespond struct {
	Total int64                       `json:"total"` // 总数量
	List  []FriendApplyListRespond    `json:"list"`  // 申请列表
}