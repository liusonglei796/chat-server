package request

// OpenSessionRequest 打开会话请求
// 使用位置:
//   - api/v1/session_controller.go: OpenSessionHandler
//   - internal/service/logic/session_service.go: OpenSession
//
// 注意: SendId 从 JWT 上下文获取，不从请求体传入，防止 IDOR 攻击
type OpenSessionRequest struct {
	ReceiveId string `json:"receive_id" binding:"required"`
}
