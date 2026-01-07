package respond

// GetGroupDetailRespond 获取群聊详情响应
// 使用位置:
//   - internal/service/contact/service.go: GetGroupDetail
type GetGroupDetailRespond struct {
	GroupId     string `json:"group_id"`
	GroupName   string `json:"group_name"`
	GroupAvatar string `json:"group_avatar"`
	GroupNotice string `json:"group_notice"`
	MemberCnt   int    `json:"member_cnt"`
	OwnerId     string `json:"owner_id"`
	AddMode     int8   `json:"add_mode"`
}
