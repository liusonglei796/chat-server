package auth

// LoginRequest 用户密码登录请求
// 使用位置:
//   - api/v1/user_info_controller.go: LoginHandler
//   - internal/service/logic/user_info_service.go: Login
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
}
