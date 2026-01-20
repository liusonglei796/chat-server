package request

// CheckSessionAllowedRequest 检查会话权限请求
// 使用位置:
//   - handler/session_handler.go: CheckOpenSessionAllowed
//
// 注意: SendId 从 JWT 上下文获取，不从请求传入，防止 IDOR 攻击
type CheckSessionAllowedRequest struct {
	ReceiveId string `json:"receive_id" form:"receive_id" binding:"required"`
}
