package request

// SmsLoginRequest 短信验证码登录请求
// 使用位置:
//   - api/v1/user_info_controller.go: SmsLoginHandler
//   - internal/service/logic/user_info_service.go: SmsLogin
type SmsLoginRequest struct {
	Telephone string `json:"telephone" binding:"required"`
	SmsCode   string `json:"sms_code" binding:"required,len=6"`
}
