package respond

// GroupSessionListRespond 群聊会话列表响应
// 使用位置:
//   - internal/service/logic/session_service.go: GetGroupSessionList
type GroupSessionListRespond struct {
	SessionId string `json:"session_id"`
	GroupName string `json:"group_name"`
	GroupId   string `json:"group_id"`
	Avatar    string `json:"avatar"`
}
