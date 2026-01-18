// Package handler 提供 HTTP 请求处理器
// 本文件定义 Handler 聚合结构和构造函数
// 遵循依赖倒置原则，通过构造函数注入 Service 依赖
package handler

import (
	"kama_chat_server/internal/service"
	"kama_chat_server/internal/service/chat"
)

// Handlers 聚合所有 Handler 实例
// 作为依赖注入的入口，Router 层通过此结构访问各个 Handler
type Handlers struct {
	User    *UserHandler
	Auth    *AuthHandler
	Contact *ContactHandler
	Apply   *ApplyHandler
	Group   *GroupHandler
	Session *SessionHandler
	Message *MessageHandler
	Ws      *WsHandler
}

// NewHandlers 创建并注入所有 Handler 实例
func NewHandlers(svc *service.Services, broker chat.MessageBroker) *Handlers {
	return &Handlers{
		User:    NewUserHandler(svc.User),
		Auth:    NewAuthHandler(svc.Auth),
		Contact: NewContactHandler(svc.Contact),
		Apply:   NewApplyHandler(svc.Apply),
		Group:   NewGroupHandler(svc.Group),
		Session: NewSessionHandler(svc.Session),
		Message: NewMessageHandler(svc.Message),
		Ws:      NewWsHandler(broker),
	}
}
