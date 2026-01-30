// Package service 定义业务层接口
// 本文件定义所有 Service 接口，供 Handler 层调用
// 接口设计遵循依赖倒置原则，便于测试和解耦
package service

import (
	"github.com/gin-gonic/gin"

	adminreq "kama_chat_server/internal/dto/request/admin"
	"kama_chat_server/internal/dto/request/apply"
	"kama_chat_server/internal/dto/request/auth"
	"kama_chat_server/internal/dto/request/group"
	"kama_chat_server/internal/dto/request/session"
	"kama_chat_server/internal/dto/request/user"

	adminrsp "kama_chat_server/internal/dto/respond/admin"
	applyrsp "kama_chat_server/internal/dto/respond/apply"
	contactrsp "kama_chat_server/internal/dto/respond/contact"
	grouprsp "kama_chat_server/internal/dto/respond/group"
	messagersp "kama_chat_server/internal/dto/respond/message"
	sessionrsp "kama_chat_server/internal/dto/respond/session"
	userrsp "kama_chat_server/internal/dto/respond/user"
)

// UserService 用户业务接口
// 处理用户注册、登录、信息管理等功能
type UserService interface {
	// Login 密码登录
	Login(req auth.LoginRequest) (*userrsp.LoginRespond, error)
	// SmsLogin 短信验证码登录
	SmsLogin(req auth.SmsLoginRequest) (*userrsp.LoginRespond, error)
	// SendSmsCode 发送短信验证码
	SendSmsCode(telephone string) error
	// Register 用户注册
	Register(req auth.RegisterRequest) (*userrsp.RegisterRespond, error)
	// UpdateUserInfo 更新用户信息 (userId 从 JWT 获取，只能改自己)
	UpdateUserInfo(userId string, req user.UpdateUserInfoRequest) error
	// GetUserInfo 获取用户完整信息（仅限自己或管理员调用）
	GetUserInfo(requesterId, targetId string) (*userrsp.GetUserInfoRespond, error)
	// GetPublicUserInfo 获取用户公开信息（查看他人）
	GetPublicUserInfo(targetId string) (*userrsp.PublicUserInfoRespond, error)
}

// SessionService 会话业务接口
// 处理聊天会话的创建、打开、删除等功能
type SessionService interface {
	// CreateSession 创建新会话
	CreateSession(sendId, receiveId string) (string, error)
	// CheckOpenSessionAllowed 检查是否允许打开会话
	CheckOpenSessionAllowed(sendId, receiveId string) (bool, error)
	// OpenSession 打开/获取会话 (sendId 从 JWT 获取，防止 IDOR)
	OpenSession(sendId string, req session.OpenSessionRequest) (string, error)
	// GetUserSessionList 获取用户单聊会话列表
	GetUserSessionList(ownerId string) ([]sessionrsp.UserSessionListRespond, error)
	// GetGroupSessionList 获取用户群聊会话列表
	GetGroupSessionList(ownerId string) ([]sessionrsp.GroupSessionListRespond, error)
	// DeleteSession 删除会话
	DeleteSession(ownerId, sessionId string) error
}

// GroupService 群组业务接口
// 处理群组的创建、管理、成员管理等功能
type GroupService interface {
	// CreateGroup 创建群组 (ownerId 从 JWT 获取)
	CreateGroup(ownerId string, req group.CreateGroupRequest) error
	// LoadMyGroup 加载我创建的群组
	LoadMyGroup(ownerId string) ([]grouprsp.MyGroupListRespond, error)
	// GetJoinedGroups 获取我加入的群组
	GetJoinedGroups(userId string) ([]grouprsp.MyGroupListRespond, error)
	// CheckGroupAddMode 检查加群方式
	CheckGroupAddMode(groupId string) (int8, error)
	// LeaveGroup 退出群组
	LeaveGroup(userId, groupId string) error
	// DismissGroup 解散群组 (operatorId 必须是群主)
	DismissGroup(operatorId, groupId string) error
	// GetPublicGroupInfo 获取群组公开信息（非群成员也可查看）
	GetPublicGroupInfo(groupId string) (*grouprsp.PublicGroupInfoRespond, error)
	// UpdateGroupInfo 更新群组信息 (operatorId 必须是群主或管理员)
	UpdateGroupInfo(operatorId string, req group.UpdateGroupInfoRequest) error
	// GetGroupMemberList 获取群成员列表 (userId 必须是群成员)
	GetGroupMemberList(userId, groupId string) ([]grouprsp.GetGroupMemberListRespond, error)
	// RemoveGroupMembers 移除群成员 (operatorId 必须是群主或管理员)
	RemoveGroupMembers(operatorId string, req group.RemoveGroupMembersRequest) error
}

// ContactService 联系人业务接口
// 处理好友关系、联系人管理等功能
type ContactService interface {
// GetUserList 获取好友列表（分页）
	GetUserList(userId string, page, pageSize int) ([]userrsp.MyUserListRespond, int64, error)
// GetGroupList 获取群组列表（分页）
	GetGroupList(userId string, page, pageSize int) ([]grouprsp.MyGroupListRespond, int64, error)

	// GetFriendInfo 获取好友详情 (userId 必须与 friendId 是好友关系)
	GetFriendInfo(userId, friendId string) (contactrsp.FriendInfoRespond, error)
	// GetGroupDetail 获取群聊详情 (userId 必须是群成员)
	GetGroupDetail(userId, groupId string) (contactrsp.GroupDetailRespond, error)
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
	ApplyFriend(userId string, req apply.ApplyFriendRequest) error
	// GetFriendApplyList 获取待处理的好友申请列表
	GetFriendApplyList(userId string) ([]applyrsp.FriendApplyListRespond, error)
	// PassFriendApply 通过好友申请
	PassFriendApply(userId, applicantId string) error
	// RefuseFriendApply 拒绝好友申请
	RefuseFriendApply(userId, applicantId string) error
	// BlackFriendApply 拉黑好友申请
	BlackFriendApply(userId, applicantId string) error

	// ===== 入群申请相关 =====
	// ApplyGroup 申请加入群组
	ApplyGroup(userId string, req apply.ApplyGroupRequest) error
	// GetGroupApplyList 获取入群申请列表
	GetGroupApplyList(userId, groupId string) ([]applyrsp.GroupApplyListRespond, error)
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
	// GetMessageList 获取两个用户之间的聊天记录 (requesterId 用于权限校验)
	GetMessageList(requesterId, partnerId string, page, pageSize int) ([]messagersp.GetMessageListRespond, error)
	// GetGroupMessageList 获取群聊消息记录 (userId 必须是群成员)
	GetGroupMessageList(userId, groupId string) ([]messagersp.GetMessageListRespond, error)
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

// ==================== 后台管理服务 ====================

// UserAdminService 用户管理后台接口
type UserAdminService interface {
	// GetUserListPaged 分页获取用户列表
	GetUserListPaged(req adminreq.GetUserListRequest) (*adminrsp.PagedUserListRespond, error)
	// BatchUpdateUserStatus 批量更新用户状态（启用/禁用/删除）
	BatchUpdateUserStatus(req adminreq.BatchUpdateUserStatusRequest) error
	// SetAdmin 批量设置管理员权限
	SetAdmin(userUUIDs []string, isAdmin int8) error
}

// GroupAdminService 群组管理后台接口
type GroupAdminService interface {
	// GetGroupInfoList 分页获取群组列表（管理员）
	GetGroupInfoList(req adminreq.GetGroupInfoListRequest) (*adminrsp.GetGroupListWrapper, error)
	// DeleteGroups 批量删除群组
	DeleteGroups(groupUUIDs []string) error
	// SetGroupsStatus 批量设置群组状态
	SetGroupsStatus(groupUUIDs []string, status int8) error
}
