package request

// DeleteSessionRequest 删除会话请求
// 使用位置:
//   - handler/session_handler.go: DeleteSessionHandler
type DeleteSessionRequest struct {
	UserId    string `json:"user_id" binding:"required"`
	SessionId string `json:"session_id" binding:"required"`
}
