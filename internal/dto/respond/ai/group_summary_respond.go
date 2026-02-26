package ai

// GroupSummaryRespond 群聊总结响应
type GroupSummaryRespond struct {
	Summary   string   `json:"summary"`
	Todos     []string `json:"todos"`
	Decisions []string `json:"decisions"`
}
