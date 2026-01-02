package request

// SendSmsCodeRequest 发送短信验证码请求
// 使用位置:
//   - api/v1/user_info_controller.go: SendSmsCodeHandler
type SendSmsCodeRequest struct {
	Telephone string `json:"telephone" binding:"required"`
}
