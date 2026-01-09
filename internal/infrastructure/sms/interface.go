// Package sms 提供短信服务
// 本文件定义短信服务接口，遵循依赖倒置原则
package sms

// SmsService 短信服务接口
// 抽象短信发送操作，支持多种实现（阿里云、腾讯云等）
// Service 层应依赖此接口而非具体实现
type SmsService interface {
	// SendVerificationCode 发送短信验证码
	// telephone: 手机号码
	// 返回: 操作错误
	SendVerificationCode(telephone string) error
}

// 确保 aliyunSmsService 实现了 SmsService 接口
var _ SmsService = (*aliyunSmsService)(nil)
