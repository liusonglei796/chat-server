// Package service 提供 Service 层聚合与构造
package service

import (
	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/infrastructure/sms"
	"kama_chat_server/internal/service/auth"
	"kama_chat_server/internal/service/contact"
	"kama_chat_server/internal/service/group"
	"kama_chat_server/internal/service/message"
	"kama_chat_server/internal/service/session"
	"kama_chat_server/internal/service/user"
)

// Services 聚合所有 Service 实例
// 作为依赖注入的入口，Handler 层通过 service.Services 访问各个 Service
type Services struct {
	User    UserService    // 用户 Service
	Session SessionService // 会话 Service
	Group   GroupService   // 群组 Service
	Contact ContactService // 联系人 Service
	Message MessageService // 消息 Service
	Auth    AuthService    // 认证 Service
}

// NewServices 创建并注入所有 Service 实例
func NewServices(repos *mysql.Repositories, cacheService myredis.AsyncCacheService, smsService sms.SmsService) *Services {
	sessionSvc := session.NewSessionService(repos, cacheService)
	userSvc := user.NewUserService(repos, cacheService, smsService)
	groupSvc := group.NewGroupService(repos, cacheService)
	contactSvc := contact.NewContactService(repos, cacheService)
	messageSvc := message.NewMessageService(repos, cacheService)
	authSvc := auth.NewAuthService(cacheService)

	return &Services{
		User:    userSvc,
		Session: sessionSvc,
		Group:   groupSvc,
		Contact: contactSvc,
		Message: messageSvc,
		Auth:    authSvc,
	}
}
