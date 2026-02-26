package message

// TranslateRespond 翻译响应
type TranslateRespond struct {
	// TranslationEnabled 是否开启翻译
	TranslationEnabled bool `json:"translationEnabled"`
	// TargetLang 目标语言
	TargetLang string `json:"targetLang"`
	// OriginalText 原文
	OriginalText string `json:"originalText"`
	// TranslatedText 译文
	TranslatedText string `json:"translatedText"`
}
