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
	// UpdateUserInfo 更新用户信息 (userId 从 JWT 获取，只能改自己)
	UpdateUserInfo(userId string, req request.UpdateUserInfoRequest) error
	// GetUserListPaged 分页获取用户列表（管理员）
	GetUserListPaged(req request.GetUserListPagedRequest) (*respond.PagedUserListRespond, error)
	// BatchUpdateUserStatus 批量更新用户状态（启用/禁用/删除）
	BatchUpdateUserStatus(req request.BatchUpdateUserStatusRequest) error
	// GetUserInfo 获取用户完整信息（仅限自己或管理员调用）
	GetUserInfo(requesterId, targetId string) (*respond.GetUserInfoRespond, error)
	// GetPublicUserInfo 获取用户公开信息（查看他人）
	GetPublicUserInfo(targetId string) (*respond.PublicUserInfoRespond, error)
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
	// CreateGroup 创建群组 (ownerId 从 JWT 获取)
	CreateGroup(ownerId string, req request.CreateGroupRequest) error
	// LoadMyGroup 加载我创建的群组
	LoadMyGroup(ownerId string) ([]respond.LoadMyGroupRespond, error)
	// CheckGroupAddMode 检查群组加入方式
	CheckGroupAddMode(groupId string) (int8, error)
	// EnterGroupDirectly 直接加入群组（无需审核）
	EnterGroupDirectly(groupId, userId string) error
	// LeaveGroup 退出群组
	LeaveGroup(userId, groupId string) error
	// DismissGroup 解散群组 (operatorId 必须是群主)
	DismissGroup(operatorId, groupId string) error
	// GetGroupInfo 获取群组信息
	GetGroupInfo(groupId string) (*respond.GetGroupInfoRespond, error)
	// GetGroupInfoList 分页获取群组列表（管理员）
	GetGroupInfoList(req request.GetGroupListRequest) (*respond.GetGroupListWrapper, error)
	// DeleteGroups 批量删除群组
	DeleteGroups(uuidList []string) error
	// SetGroupsStatus 批量设置群组状态
	SetGroupsStatus(uuidList []string, status int8) error
	// UpdateGroupInfo 更新群组信息 (operatorId 必须是群主或管理员)
	UpdateGroupInfo(operatorId string, req request.UpdateGroupInfoRequest) error
	// GetGroupMemberList 获取群成员列表 (userId 必须是群成员)
	GetGroupMemberList(userId, groupId string) ([]respond.GetGroupMemberListRespond, error)
	// RemoveGroupMembers 移除群成员 (operatorId 必须是群主或管理员)
	RemoveGroupMembers(operatorId string, req request.RemoveGroupMembersRequest) error
}

// ContactService 联系人业务接口
// 处理好友关系、联系人管理等功能
type ContactService interface {
	// GetUserList 获取用户的好友列表
	GetUserList(userId string) ([]respond.MyUserListRespond, error)
	// GetJoinedGroupsExcludedOwn 获取已加入的群组（排除自己创建的）
	GetJoinedGroupsExcludedOwn(userId string) ([]respond.LoadMyJoinedGroupRespond, error)
	// GetFriendInfo 获取好友详情 (userId 必须与 friendId 是好友关系)
	GetFriendInfo(userId, friendId string) (respond.GetFriendInfoRespond, error)
	// GetGroupDetail 获取群聊详情 (userId 必须是群成员)
	GetGroupDetail(userId, groupId string) (respond.GetGroupDetailRespond, error)
	// DeleteContact 删除联系人
	DeleteContact(userId, contactId string) error
	// BlackContact 拉黑联系人
	BlackContact(userId, contactId string) error
	// CancelBlackContact 取消拉黑
	CancelBlackContact(userId, contactId string) error
}

// ApplyService 申请业务接口
// 处理好友申请、入群申请等功能
type ApplyService interface {
	// ===== 好友申请相关 =====
	// ApplyFriend 申请添加好友
	ApplyFriend(userId string, req request.ApplyFriendRequest) error
	// GetFriendApplyList 获取待处理的好友申请列表
	GetFriendApplyList(userId string) ([]respond.NewContactListRespond, error)
	// PassFriendApply 通过好友申请
	PassFriendApply(userId, applicantId string) error
	// RefuseFriendApply 拒绝好友申请
	RefuseFriendApply(userId, applicantId string) error
	// BlackFriendApply 拉黑好友申请
	BlackFriendApply(userId, applicantId string) error

	// ===== 入群申请相关 =====
	// ApplyGroup 申请加入群组
	ApplyGroup(userId string, req request.ApplyGroupRequest) error
	// GetGroupApplyList 获取入群申请列表
	GetGroupApplyList(userId, groupId string) ([]respond.AddGroupListRespond, error)
	// PassGroupApply 通过入群申请 (operatorId 需要是群主或管理员)
	PassGroupApply(operatorId, groupId, applicantId string) error
	// RefuseGroupApply 拒绝入群申请 (operatorId 需要是群主或管理员)
	RefuseGroupApply(operatorId, groupId, applicantId string) error
	// BlackGroupApply 拉黑入群申请 (operatorId 需要是群主或管理员)
	BlackGroupApply(operatorId, groupId, applicantId string) error
}

// MessageService 消息业务接口
// 处理消息历史记录和文件上传等功能
type MessageService interface {
	// GetMessageList 获取两个用户之间的聊天记录
	GetMessageList(userOneId, userTwoId string) ([]respond.GetMessageListRespond, error)
	// GetGroupMessageList 获取群聊消息记录 (userId 必须是群成员)
	GetGroupMessageList(userId, groupId string) ([]respond.GetGroupMessageListRespond, error)
	// UploadAvatar 上传头像，返回新文件名
	UploadAvatar(c *gin.Context) (string, error)
	// UploadFile 上传文件，返回文件名列表
	UploadFile(c *gin.Context) ([]string, error)
}

// AuthService 认证业务接口
// 处理 Token 刷新和验证等功能
type AuthService interface {
	// ValidateTokenID 验证用户的 Token ID 是否有效（用于单点登录互踢）
	// userID: 用户ID
	// tokenID: 需要验证的 Token ID
	// 返回: 是否有效, 错误信息
	ValidateTokenID(userID, tokenID string) (bool, error)
}
