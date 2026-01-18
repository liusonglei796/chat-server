package respond

// PublicUserInfoRespond 公开用户信息响应（查询他人时返回）
// 使用位置:
//   - internal/service/user/service.go: GetPublicUserInfo
//
// 不包含敏感字段: telephone, email, is_admin, status
type PublicUserInfoRespond struct {
	Uuid      string `json:"uuid"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	Gender    int8   `json:"gender"`
	Birthday  string `json:"birthday"`
	Signature string `json:"signature"`
}
