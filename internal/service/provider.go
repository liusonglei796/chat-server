package service

import (
	"kama_chat_server/internal/dao/mysql/repository"
	"kama_chat_server/internal/service/chatroom"
	"kama_chat_server/internal/service/contact"
	"kama_chat_server/internal/service/group"
	"kama_chat_server/internal/service/message"
	"kama_chat_server/internal/service/session"
	"kama_chat_server/internal/service/user"
)

// Services 聚合所有 Service 实例
type Services struct {
	User     UserService
	Session  SessionService
	Group    GroupService
	Contact  ContactService
	Message  MessageService
	ChatRoom ChatRoomService
}

// NewServices 创建并注入所有 Service 实例
func NewServices(repos *repository.Repositories) *Services {
	sessionSvc := session.NewSessionService(repos)

	userSvc := user.NewUserService(repos)

	groupSvc := group.NewGroupService(repos)

	contactSvc := contact.NewContactService(repos)

	messageSvc := message.NewMessageService(repos)

	chatRoomSvc := chatroom.NewChatRoomService()

	return &Services{
		User:     userSvc,
		Session:  sessionSvc,
		Group:    groupSvc,
		Contact:  contactSvc,
		Message:  messageSvc,
		ChatRoom: chatRoomSvc,
	}
}

// Svc 全局 Services 实例
var Svc *Services

// InitServices 初始化全局 Services 实例
func InitServices(repos *repository.Repositories) {
	Svc = NewServices(repos)
}
