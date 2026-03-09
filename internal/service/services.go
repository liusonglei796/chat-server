// Package service 提供 Service 层聚合与构造
package service

import (
	"kama_chat_server/internal/config"
	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/infrastructure/sms"
	admingroup "kama_chat_server/internal/service/admin/group"
	adminuser "kama_chat_server/internal/service/admin/user"
	"kama_chat_server/internal/service/apply"
	"kama_chat_server/internal/service/auth"
	"kama_chat_server/internal/service/friendship"
	"kama_chat_server/internal/service/group"
	"kama_chat_server/internal/service/message"
	"kama_chat_server/internal/service/session"
	"kama_chat_server/internal/service/user"
)

// Services 聚合所有 Service 实例
// 作为依赖注入的入口，Handler 层通过 service.Services 访问各个 Service
type Services struct {
	User       *user.UserService             // 用户 Service
	Session    *session.SessionService       // 会话 Service
	Group      *group.GroupService           // 群组 Service
	Friendship *friendship.FriendshipService // 好友关系 Service
	Apply      *apply.ApplyService           // 申请 Service
	Message    *message.MessageService       // 消息 Service
	Auth       *auth.Service                 // 认证 Service

	// 后台管理 Services
	UserAdmin  *adminuser.UserAdminService   // 用户管理后台 Service
	GroupAdmin *admingroup.GroupAdminService // 群组管理后台 Service
}

// NewServices 创建并注入所有 Service 实例
// uow: UnitOfWork 接口，提供事务支持和 Repository 访问
// kickClient: 可选的踢人回调函数，由 ChatServer.Broker.KickClient 提供
// pushRecallNotify: 可选的撤回通知回调，由 ChatServer.Broker.PushRecallNotify 提供
func NewServices(uow repository.UnitOfWork, cacheService repository.AsyncCacheService, smsService sms.SmsService, kickClient func(userId, reason string), pushRecallNotify func(messageUuid, receiveId string), cfg *config.Config) *Services {
	// 非事务型 Service：注入单独的 Repository 接口
	sessionSvc := session.NewSessionService(
		uow.SessionRepo(), uow.UserRepo(), uow.GroupRepo(),
		uow.GroupMemberRepo(), uow.FriendshipRepo(), uow.MessageRepo(),
		cacheService,
	)
	messageSvc := message.NewMessageService(
		uow.MessageRepo(), uow.FriendshipRepo(), uow.SessionRepo(),
		cacheService, pushRecallNotify,
	)

	// 事务型 Service：注入 UnitOfWork 接口
	userSvc := user.NewUserService(uow, cacheService, smsService, kickClient)
	groupSvc := group.NewGroupService(uow, cacheService)
	friendshipSvc := friendship.NewFriendshipService(uow, cacheService)
	applySvc := apply.NewApplyService(uow, cacheService)
	authSvc := auth.NewAuthService(cacheService, uow.UserRepo())

	// 后台管理服务（事务型）
	userAdminSvc := adminuser.NewUserAdminService(uow, cacheService)
	groupAdminSvc := admingroup.NewGroupAdminService(uow, cacheService)

	return &Services{
		User:       userSvc,
		Session:    sessionSvc,
		Group:      groupSvc,
		Friendship: friendshipSvc,
		Apply:      applySvc,
		Message:    messageSvc,
		Auth:       authSvc,
		UserAdmin:  userAdminSvc,
		GroupAdmin: groupAdminSvc,
	}
}
