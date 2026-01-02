package respond

// LoadMyJoinedGroupRespond 我加入的群聊列表响应
// 使用位置:
//   - internal/service/logic/user_contact_service.go: LoadMyJoinedGroup
type LoadMyJoinedGroupRespond struct {
	GroupId   string `json:"group_id"`
	GroupName string `json:"group_name"`
	Avatar    string `json:"avatar"`
}
