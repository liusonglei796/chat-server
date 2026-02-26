package message

// TranslateRespond 翻译响应
type TranslateRespond struct {
	// OriginalText 原文
	OriginalText string `json:"originalText"`
	// TranslatedText 译文
	TranslatedText string `json:"translatedText"`
}
