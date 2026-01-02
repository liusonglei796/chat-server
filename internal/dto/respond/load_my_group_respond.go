package respond

// LoadMyGroupRespond 我创建的群聊列表响应
// 使用位置:
//   - internal/service/logic/group_info_service.go: LoadMyGroup
type LoadMyGroupRespond struct {
	GroupId   string `json:"group_id"`
	GroupName string `json:"group_name"`
	Avatar    string `json:"avatar"`
}
