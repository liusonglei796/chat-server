// Package handler 提供 HTTP 请求处理器
// 本文件定义 Handler 聚合结构和构造函数
// 遵循依赖倒置原则，通过 gRPC 客户端去调用微服务
package handler

import (
	"kama_chat_server/internal/apps/message/chat"
)

// Handlers 聚合所有 Handler 实例
// 作为依赖注入的入口，Router 层通过此结构访问各个 Handler
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
func NewHandlers(broker *chat.MsgConsumer) *Handlers {
	return &Handlers{
		User:       NewUserHandler(),
		Auth:       NewAuthHandler(),
		Friendship: NewFriendshipHandler(),
		Apply:      NewApplyHandler(),
		Group:      NewGroupHandler(),
		Session:    NewSessionHandler(),
		Message:    NewMessageHandler(),
		Ws:         NewWsHandler(broker),
	}
}
