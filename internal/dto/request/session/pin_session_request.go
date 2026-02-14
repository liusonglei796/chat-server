package session

// PinSessionRequest 会话置顶请求
type PinSessionRequest struct {
	// SessionId 会话 UUID
	SessionId string `json:"session_id" binding:"required"`
	// IsPinned 是否置顶
	IsPinned bool `json:"is_pinned"`
}
