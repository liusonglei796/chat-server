package request

// RegisterRequest 用户注册请求
// 使用位置:
//   - api/v1/user_info_controller.go: RegisterHandler
//   - internal/service/logic/user_info_service.go: Register
type RegisterRequest struct {
	Telephone string `json:"telephone" binding:"required"`
	Password  string `json:"password" binding:"required,min=6"`
	Nickname  string `json:"nickname" binding:"required"`
	SmsCode   string `json:"sms_code" binding:"required,len=6"`
}
