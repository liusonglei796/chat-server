// Package service 定义业务层接口
// 本文件定义所有 Service 接口，供 Handler 层调用
// 接口设计遵循依赖倒置原则，便于测试和解耦
package service

import (
	"github.com/gin-gonic/gin"

	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/dto/respond"
)

// UserService 用户业务接口
// 处理用户注册、登录、信息管理等功能
type UserService interface {
	// Login 密码登录
	Login(req request.LoginRequest) (*respond.LoginRespond, error)
	// SmsLogin 短信验证码登录
	SmsLogin(req request.SmsLoginRequest) (*respond.LoginRespond, error)
	// SendSmsCode 发送短信验证码
	SendSmsCode(telephone string) error
	// Register 用户注册
	Register(req request.RegisterRequest) (*respond.RegisterRespond, error)
	// UpdateUserInfo 更新用户信息
	UpdateUserInfo(req request.UpdateUserInfoRequest) error
	// GetUserInfoList 获取用户列表（排除指定用户）
	GetUserInfoList(ownerId string) ([]respond.GetUserListRespond, error)
	// AbleUsers 批量启用用户
	AbleUsers(uuidList []string) error
	// DisableUsers 批量禁用用户
	DisableUsers(uuidList []string) error
	// DeleteUsers 批量删除用户（软删除）
	DeleteUsers(uuidList []string) error
	// GetUserInfo 获取单个用户信息
	GetUserInfo(uuid string) (*respond.GetUserInfoRespond, error)
	// SetAdmin 批量设置管理员权限
	SetAdmin(uuidList []string, isAdmin int8) error
}

// SessionService 会话业务接口
// 处理聊天会话的创建、打开、删除等功能
type SessionService interface {
	// CreateSession 创建新会话
	CreateSession(req request.CreateSessionRequest) (string, error)
	// CheckOpenSessionAllowed 检查是否允许打开会话
	CheckOpenSessionAllowed(sendId, receiveId string) (bool, error)
	// OpenSession 打开/获取会话
	OpenSession(req request.OpenSessionRequest) (string, error)
	// GetUserSessionList 获取用户单聊会话列表
	GetUserSessionList(ownerId string) ([]respond.UserSessionListRespond, error)
	// GetGroupSessionList 获取用户群聊会话列表
	GetGroupSessionList(ownerId string) ([]respond.GroupSessionListRespond, error)
	// DeleteSession 删除会话
	DeleteSession(ownerId, sessionId string) error
}

// GroupService 群组业务接口
// 处理群组的创建、管理、成员管理等功能
type GroupService interface {
	// CreateGroup 创建群组
	CreateGroup(req request.CreateGroupRequest) error
	// LoadMyGroup 加载我创建的群组
	LoadMyGroup(ownerId string) ([]respond.LoadMyGroupRespond, error)
	// CheckGroupAddMode 检查群组加入方式
	CheckGroupAddMode(groupId string) (int8, error)
	// EnterGroupDirectly 直接加入群组（无需审核）
	EnterGroupDirectly(groupId, userId string) error
	// LeaveGroup 退出群组
	LeaveGroup(userId, groupId string) error
	// DismissGroup 解散群组
	DismissGroup(ownerId, groupId string) error
	// GetGroupInfo 获取群组信息
	GetGroupInfo(groupId string) (*respond.GetGroupInfoRespond, error)
	// GetGroupInfoList 分页获取群组列表（管理员）
	GetGroupInfoList(req request.GetGroupListRequest) (*respond.GetGroupListWrapper, error)
	// DeleteGroups 批量删除群组
	DeleteGroups(uuidList []string) error
	// SetGroupsStatus 批量设置群组状态
	SetGroupsStatus(uuidList []string, status int8) error
	// UpdateGroupInfo 更新群组信息
	UpdateGroupInfo(req request.UpdateGroupInfoRequest) error
	// GetGroupMemberList 获取群成员列表
	GetGroupMemberList(groupId string) ([]respond.GetGroupMemberListRespond, error)
	// RemoveGroupMembers 移除群成员
	RemoveGroupMembers(req request.RemoveGroupMembersRequest) error
}

// ContactService 联系人业务接口
// 处理好友关系、联系人申请等功能
type ContactService interface {
	// GetUserList 获取用户的好友列表
	GetUserList(userId string) ([]respond.MyUserListRespond, error)
	// GetJoinedGroupsExcludedOwn 获取已加入的群组（排除自己创建的）
	GetJoinedGroupsExcludedOwn(userId string) ([]respond.LoadMyJoinedGroupRespond, error)
	// GetContactInfo 获取联系人信息
	GetContactInfo(contactId string) (respond.GetContactInfoRespond, error)
	// DeleteContact 删除联系人
	DeleteContact(userId, contactId string) error
	// ApplyContact 申请添加联系人
	ApplyContact(req request.ApplyContactRequest) error
	// GetNewContactList 获取新联系人申请列表
	GetNewContactList(userId string) ([]respond.NewContactListRespond, error)
	// GetAddGroupList 获取入群申请列表
	GetAddGroupList(groupId string) ([]respond.AddGroupListRespond, error)
	// PassContactApply 通过联系人申请
	PassContactApply(targetId, applicantId string) error
	// RefuseContactApply 拒绝联系人申请
	RefuseContactApply(targetId, applicantId string) error
	// BlackContact 拉黑联系人
	BlackContact(userId, contactId string) error
	// CancelBlackContact 取消拉黑
	CancelBlackContact(userId, contactId string) error
	// BlackApply 拉黑申请（不再接受该用户的申请）
	BlackApply(targetId, applicantId string) error
}

// MessageService 消息业务接口
// 处理消息历史记录和文件上传等功能
type MessageService interface {
	// GetMessageList 获取两个用户之间的聊天记录
	GetMessageList(userOneId, userTwoId string) ([]respond.GetMessageListRespond, error)
	// GetGroupMessageList 获取群聊消息记录
	GetGroupMessageList(groupId string) ([]respond.GetGroupMessageListRespond, error)
	// UploadAvatar 上传头像，返回新文件名
	UploadAvatar(c *gin.Context) (string, error)
	// UploadFile 上传文件，返回文件名列表
	UploadFile(c *gin.Context) ([]string, error)
}

// ChatRoomService 聊天室业务接口
// 处理聊天室相关功能
type ChatRoomService interface {
	// GetCurContactListInChatRoom 获取当前聊天室中的联系人列表
	GetCurContactListInChatRoom(userId, contactId string) ([]respond.GetCurContactListInChatRoomRespond, error)
}
