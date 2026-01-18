package request

// DeleteSessionRequest 删除会话请求
// 使用位置:
//   - handler/session_handler.go: DeleteSessionHandler
//
// 注意: UserId 从 JWT 上下文获取，不从请求体读取
type DeleteSessionRequest struct {
	SessionId string `json:"session_id" binding:"required"`
}
