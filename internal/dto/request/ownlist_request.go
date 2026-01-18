package request

// OwnlistRequest 通用列表请求（无需用户ID，从JWT上下文获取）
type OwnlistRequest struct {
	// UserId 从JWT中间件上下文获取，不再需要客户端传递
}
