package user

// GetUserInfoRequest 获取用户信息请求
// 使用位置: handler/user_handler.go: GetUserInfo
type GetUserInfoRequest struct {
	Uuid string `json:"uuid" form:"uuid" binding:"required"` // 用户UUID
}