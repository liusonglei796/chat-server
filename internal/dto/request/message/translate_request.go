package message

// TranslateRequest 翻译请求
type TranslateRequest struct {
	// MessageID 消息ID
	MessageID string `json:"messageId" binding:"required"`
	// EnableTranslate 是否开启翻译（nil 代表兼容旧版本，默认开启）
	EnableTranslate *bool `json:"enableTranslate"`
	// TargetLang 目标语言，默认英语
	TargetLang string `json:"targetLang"`
}
