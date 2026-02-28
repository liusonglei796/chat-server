package constants

import "time"

// AI 服务相关常量
const (
	// 智能回复
	DefaultReplyContextLimit = 20 // 默认上下文消息条数
	MaxReplyContextLimit     = 50 // 最大上下文消息条数

	// 群聊总结
	DefaultSummaryHours = 24  // 默认总结过去 24 小时
	DefaultSummaryLimit = 200 // 默认拉取 200 条消息
	MaxSummaryLimit     = 500 // 最大拉取 500 条消息

	// 通用
	AIRequestTimeout = 12 * time.Second // AI 请求超时时间
)
