package message_type

// 消息类型
// Text: 文本
// Voice: 语音
// File: 文件
// AudioOrVideo: 音视频通话
// KickNotification: 设备下线通知
const (
	Text = iota
	// 语音
	Voice
	// 文件
	File
	// 通话
	AudioOrVideo
	// 设备下线通知（单点登录互踢）
	KickNotification = 99
)
