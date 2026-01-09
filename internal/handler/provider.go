// Package handler 提供 HTTP 请求处理器
// 本文件定义 Handler 聚合结构和构造函数
// 遵循依赖倒置原则，通过构造函数注入 Service 依赖
package handler

import (
	"kama_chat_server/internal/service"
)

// Handlers 聚合所有 Handler 实例
// 作为依赖注入的入口，Router 层通过此结构访问各个 Handler
type Handlers struct {
	User    *UserHandler
	Auth    *AuthHandler
	Contact *ContactHandler
	Group   *GroupHandler
	Session *SessionHandler
	Message *MessageHandler
}

// NewHandlers 创建并注入所有 Handler 实例
// 依赖注入流程：
//  1. 接收 Services 聚合实例
//  2. 创建各个 Handler 实例，注入对应的 Service
//  3. 返回 Handlers 聚合
//
// svc: Service 层聚合实例
// 返回: Handlers 聚合指针
func NewHandlers(svc *service.Services) *Handlers {
	return &Handlers{
		User:    NewUserHandler(svc.User),
		Auth:    NewAuthHandler(svc.Auth),
		Contact: NewContactHandler(svc.Contact),
		Group:   NewGroupHandler(svc.Group),
		Session: NewSessionHandler(svc.Session),
		Message: NewMessageHandler(svc.Message),
	}
}
