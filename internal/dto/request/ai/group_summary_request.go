package ai

// GroupSummaryRequest 群聊总结请求
type GroupSummaryRequest struct {
	GroupId string `json:"group_id" binding:"required"` // 群ID
	Hours   int    `json:"hours"`                       // 时间范围（小时），默认24
	Limit   int    `json:"limit"`                       // 最大消息条数，默认200，最大500
	Style   string `json:"style"`                       // 总结风格：brief/detail
}
