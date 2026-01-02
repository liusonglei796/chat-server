package respond

// GetGroupInfoRespond 获取群聊信息响应
// 使用位置:
//   - internal/service/logic/group_info_service.go: GetGroupInfo, CheckGroupAddMode
type GetGroupInfoRespond struct {
	Uuid      string `json:"uuid"`
	Name      string `json:"name"`
	Notice    string `json:"notice"`
	MemberCnt int    `json:"member_cnt"`
	OwnerId   string `json:"owner_id"`
	AddMode   int8   `json:"add_mode"`
	Status    int8   `json:"status"`
	Avatar    string `json:"avatar"`
	IsDeleted bool   `json:"is_deleted"`
}
