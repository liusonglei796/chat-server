package session

// UserSessionListRespond 用户会话列表响应
// 使用位置:
//   - internal/service/logic/session_service.go: GetUserSessionList
type UserSessionListRespond struct {
	SessionId       string `json:"session_id"`
	Avatar          string `json:"avatar"`
	UserId          string `json:"user_id"`
	Username        string `json:"user_name"`
	LastMessage     string `json:"last_message"`
	LastMessageTime string `json:"last_message_time"`
	LastMessageType int8   `json:"last_message_type"`
}
