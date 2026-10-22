package gateway

// Handlers 聚合所有 Handler 实例
type Handlers struct {
	User       *UserHandler
	Auth       *AuthHandler
	Friendship *FriendshipHandler
	Apply      *ApplyHandler
	Group      *GroupHandler
	Session    *SessionHandler
	Message    *MessageHandler
	Ws         *WsHandler
}

// NewHandlers 创建并注入所有 Handler 实例
func NewHandlers(hub *ConnHub) *Handlers {
	return &Handlers{
		User:       NewUserHandler(),
		Auth:       NewAuthHandler(),
		Friendship: NewFriendshipHandler(),
		Apply:      NewApplyHandler(),
		Group:      NewGroupHandler(),
		Session:    NewSessionHandler(),
		Message:    NewMessageHandler(),
		Ws:         NewWsHandler(hub),
	}
}
