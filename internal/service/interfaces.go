package service

import (
	"github.com/gin-gonic/gin"

	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/dto/respond"
)

// UserService 用户业务接口
type UserService interface {
	Login(req request.LoginRequest) (*respond.LoginRespond, error)
	SmsLogin(req request.SmsLoginRequest) (*respond.LoginRespond, error)
	SendSmsCode(telephone string) error
	Register(req request.RegisterRequest) (*respond.RegisterRespond, error)
	UpdateUserInfo(req request.UpdateUserInfoRequest) error
	GetUserInfoList(ownerId string) ([]respond.GetUserListRespond, error)
	AbleUsers(uuidList []string) error
	DisableUsers(uuidList []string) error
	DeleteUsers(uuidList []string) error
	GetUserInfo(uuid string) (*respond.GetUserInfoRespond, error)
	SetAdmin(uuidList []string, isAdmin int8) error
}

// SessionService 会话业务接口
type SessionService interface {
	CreateSession(req request.CreateSessionRequest) (string, error)
	CheckOpenSessionAllowed(sendId, receiveId string) (bool, error)
	OpenSession(req request.OpenSessionRequest) (string, error)
	GetUserSessionList(ownerId string) ([]respond.UserSessionListRespond, error)
	GetGroupSessionList(ownerId string) ([]respond.GroupSessionListRespond, error)
	DeleteSession(ownerId, sessionId string) error
}

// GroupService 群组业务接口
type GroupService interface {
	CreateGroup(req request.CreateGroupRequest) error
	LoadMyGroup(ownerId string) ([]respond.LoadMyGroupRespond, error)
	CheckGroupAddMode(groupId string) (int8, error)
	EnterGroupDirectly(groupId, userId string) error
	LeaveGroup(userId, groupId string) error
	DismissGroup(ownerId, groupId string) error
	GetGroupInfo(groupId string) (*respond.GetGroupInfoRespond, error)
	GetGroupInfoList(req request.GetGroupListRequest) (*respond.GetGroupListWrapper, error)
	DeleteGroups(uuidList []string) error
	SetGroupsStatus(uuidList []string, status int8) error
	UpdateGroupInfo(req request.UpdateGroupInfoRequest) error
	GetGroupMemberList(groupId string) ([]respond.GetGroupMemberListRespond, error)
	RemoveGroupMembers(req request.RemoveGroupMembersRequest) error
}

// ContactService 联系人业务接口
type ContactService interface {
	GetUserList(userId string) ([]respond.MyUserListRespond, error)
	LoadMyJoinedGroup(userId string) ([]respond.LoadMyJoinedGroupRespond, error)
	GetContactInfo(contactId string) (respond.GetContactInfoRespond, error)
	DeleteContact(userId, contactId string) error
	ApplyContact(req request.ApplyContactRequest) error
	GetNewContactList(userId string) ([]respond.NewContactListRespond, error)
	GetAddGroupList(groupId string) ([]respond.AddGroupListRespond, error)
	PassContactApply(targetId, applicantId string) error
	RefuseContactApply(targetId, applicantId string) error
	BlackContact(userId, contactId string) error
	CancelBlackContact(userId, contactId string) error
	BlackApply(targetId, applicantId string) error
}

// MessageService 消息业务接口
type MessageService interface {
	GetMessageList(userOneId, userTwoId string) ([]respond.GetMessageListRespond, error)
	GetGroupMessageList(groupId string) ([]respond.GetGroupMessageListRespond, error)
	UploadAvatar(c *gin.Context) (string, error)
	UploadFile(c *gin.Context) ([]string, error)
}

// ChatRoomService 聊天室业务接口
type ChatRoomService interface {
	GetCurContactListInChatRoom(userId, contactId string) ([]respond.GetCurContactListInChatRoomRespond, error)
}
