package ai

// TranslateRequest 多语言翻译请求
type TranslateRequest struct {
	Text       string `json:"text" binding:"required"`        // 原文
	SourceLang string `json:"source_lang"`                    // 源语言（可空，自动识别）
	TargetLang string `json:"target_lang" binding:"required"` // 目标语言，如 zh-CN/en/ja
}
