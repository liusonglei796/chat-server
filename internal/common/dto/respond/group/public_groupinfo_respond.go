package group

// PublicGroupInfoRespond 公开群组信息响应
// 使用位置:
//   - internal/service/group/service.go: GetGroupDetail
//
// 返回群组的基本公开信息
type PublicGroupInfoRespond struct {
	GroupId     string `json:"group_id"`
	GroupName   string `json:"group_name"`
	GroupAvatar string `json:"group_avatar"`
	GroupNotice string `json:"group_notice"`
	MemberCnt   int    `json:"member_cnt"`
	OwnerId     string `json:"owner_id"`
	AddMode     int8   `json:"add_mode"`
}
