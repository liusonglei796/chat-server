// Package service 提供 Service 层聚合与构造
package service

import (
	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/infrastructure/sms"
	admingroup "kama_chat_server/internal/service/admin/group"
	adminuser "kama_chat_server/internal/service/admin/user"
	"kama_chat_server/internal/service/apply"
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
	Apply   ApplyService   // 申请 Service
	Message MessageService // 消息 Service
	Auth    AuthService    // 认证 Service

	// 后台管理 Services
	UserAdmin  UserAdminService  // 用户管理后台 Service
	GroupAdmin GroupAdminService // 群组管理后台 Service
}

// NewServices 创建并注入所有 Service 实例
// kickClient: 可选的踢人回调函数，由 ChatServer.Broker.KickClient 提供
func NewServices(repos *mysql.Repositories, cacheService myredis.AsyncCacheService, smsService sms.SmsService, kickClient func(userId, reason string)) *Services {
	sessionSvc := session.NewSessionService(repos, cacheService)
	userSvc := user.NewUserService(repos, cacheService, smsService, kickClient)
	groupSvc := group.NewGroupService(repos, cacheService)
	contactSvc := contact.NewContactService(repos, cacheService)
	applySvc := apply.NewApplyService(repos, cacheService)
	messageSvc := message.NewMessageService(repos, cacheService)
	authSvc := auth.NewAuthService(cacheService, repos.User)

	// 后台管理服务
	userAdminSvc := adminuser.NewUserAdminService(repos, cacheService)
	groupAdminSvc := admingroup.NewGroupAdminService(repos, cacheService)

	return &Services{
		User:       userSvc,
		Session:    sessionSvc,
		Group:      groupSvc,
		Contact:    contactSvc,
		Apply:      applySvc,
		Message:    messageSvc,
		Auth:       authSvc,
		UserAdmin:  userAdminSvc,
		GroupAdmin: groupAdminSvc,
	}
}
