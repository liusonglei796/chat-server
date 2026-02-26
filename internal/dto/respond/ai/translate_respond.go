package ai

// TranslateRespond 多语言翻译响应
type TranslateRespond struct {
	DetectedLang   string `json:"detected_lang"`
	TranslatedText string `json:"translated_text"`
}
