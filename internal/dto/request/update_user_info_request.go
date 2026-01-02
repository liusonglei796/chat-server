package request

// UpdateUserInfoRequest 更新用户信息请求
// 使用位置:
//   - api/v1/user_info_controller.go: UpdateUserInfoHandler
//   - internal/service/logic/user_info_service.go: UpdateUserInfo
type UpdateUserInfoRequest struct {
	Uuid      string `json:"uuid" binding:"required"`
	Email     string `json:"email"`
	Nickname  string `json:"nickname"`
	Birthday  string `json:"birthday"`
	Signature string `json:"signature"`
	Avatar    string `json:"avatar"`
}
