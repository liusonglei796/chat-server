package respond

// PublicGroupInfoRespond 公开群组信息响应（非群成员查询时返回）
// 使用位置:
//   - internal/service/group/service.go: GetPublicGroupInfo
//
// 不包含敏感字段: status, is_deleted
type PublicGroupInfoRespond struct {
	Uuid      string `json:"uuid"`
	Name      string `json:"name"`
	Notice    string `json:"notice"`
	Avatar    string `json:"avatar"`
	MemberCnt int    `json:"member_cnt"`
	AddMode   int8   `json:"add_mode"`
}
