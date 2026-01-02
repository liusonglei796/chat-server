package request

// CreateSessionRequest 创建会话请求
// 使用位置:
//   - api/v1/session_controller.go: CreateSessionHandler
//   - internal/service/logic/session_service.go: CreateSession, OpenSession
type CreateSessionRequest struct {
	SendId    string `json:"send_id" form:"send_id" binding:"required"`
	ReceiveId string `json:"receive_id" form:"receive_id" binding:"required"`
}
