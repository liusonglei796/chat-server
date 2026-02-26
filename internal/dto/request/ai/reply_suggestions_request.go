package ai

// ReplySuggestionsRequest 智能回复建议请求
type ReplySuggestionsRequest struct {
	TargetId     string `json:"target_id" binding:"required"` // 聊天目标ID（U开头私聊/G开头群聊）
	Style        string `json:"style"`                        // 风格：brief/polite/business
	Draft        string `json:"draft"`                        // 用户草稿（可选）
	ContextLimit int    `json:"context_limit"`                // 上下文消息条数，默认20，最大50
}
