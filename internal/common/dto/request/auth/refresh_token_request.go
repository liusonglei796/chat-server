package auth

// RefreshTokenRequest 刷新 Token 请求
// 使用位置: handler/auth_handler.go: RefreshToken
//
// 说明: 使用 RefreshToken 获取新的 AccessToken，延长用户登录状态
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"` // 刷新令牌
}
