package request

// EnterGroupDirectlyRequest 直接加入群组请求
// 注意: UserId 从 JWT 上下文获取，不从请求体读取
type EnterGroupDirectlyRequest struct {
	GroupId string `json:"group_id" binding:"required"`
}
