package request

// OpenSessionRequest 打开会话请求
// 使用位置:
//   - api/v1/session_controller.go: OpenSessionHandler
//   - internal/service/logic/session_service.go: OpenSession
type OpenSessionRequest struct {
	SendId    string `json:"send_id" binding:"required"`
	ReceiveId string `json:"receive_id" binding:"required"`
}
