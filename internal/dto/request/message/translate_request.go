package message

// TranslateRequest 翻译请求
type TranslateRequest struct {
	// MessageID 消息ID
	MessageID string `json:"messageId" binding:"required"`
	// TargetLang 目标语言，默认英语
	TargetLang string `json:"targetLang"`
}
