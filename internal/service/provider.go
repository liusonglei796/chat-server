// Package service 提供业务逻辑层
// 本文件实现 Service 层的依赖注入和聚合
package service

import (
	"kama_chat_server/internal/dao/mysql/repository"
	"kama_chat_server/internal/service/contact"
	"kama_chat_server/internal/service/group"
	"kama_chat_server/internal/service/message"
	"kama_chat_server/internal/service/session"
	"kama_chat_server/internal/service/user"
)

// Services 聚合所有 Service 实例
// 作为依赖注入的入口，Handler 层通过 service.Svc 访问各个 Service
type Services struct {
	User    UserService    // 用户 Service
	Session SessionService // 会话 Service
	Group   GroupService   // 群组 Service
	Contact ContactService // 联系人 Service
	Message MessageService // 消息 Service
}

// NewServices 创建并注入所有 Service 实例
// 依赖注入流程：
//  1. 接收 Repository 聚合实例
//  2. 创建各个 Service 实例，注入 Repository 依赖
//  3. 返回 Services 聚合
//
// repos: Repository 层聚合实例
// 返回: Services 聚合指针
func NewServices(repos *repository.Repositories) *Services {
	// 创建各个 Service 实例
	sessionSvc := session.NewSessionService(repos)
	userSvc := user.NewUserService(repos)
	groupSvc := group.NewGroupService(repos)
	contactSvc := contact.NewContactService(repos)
	messageSvc := message.NewMessageService(repos)

	// 聚合并返回
	return &Services{
		User:    userSvc,
		Session: sessionSvc,
		Group:   groupSvc,
		Contact: contactSvc,
		Message: messageSvc,
	}
}

// Svc 全局 Services 实例
// Handler 层通过 service.Svc.User.Login() 等方式调用
var Svc *Services

// InitServices 初始化全局 Services 实例
// 应在 main.go 中调用，在 Repository 初始化之后
// repos: Repository 层聚合实例
func InitServices(repos *repository.Repositories) {
	Svc = NewServices(repos)
}
