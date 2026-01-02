package respond

// MyUserListRespond 我的联系人用户列表响应
// 使用位置:
//   - internal/service/logic/user_contact_service.go: GetUserList
type MyUserListRespond struct {
	UserId   string `json:"user_id"`
	UserName string `json:"user_name"`
	Avatar   string `json:"avatar"`
}
