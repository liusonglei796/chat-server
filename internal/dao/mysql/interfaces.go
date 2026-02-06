// Package mysql 定义数据访问层接口和聚合结构
// 采用 Repository 模式将数据访问逻辑与业务逻辑分离
// 所有 Repository 接口在此文件定义，具体实现在各自的模块中
package mysql

import (
	"kama_chat_server/internal/model"
	"time"
)

// ==================== Repository 接口定义 ====================

// UserRepository 用户数据访问接口
// 提供用户的增删改查操作
type UserRepository interface {
	// FindByUuid 根据 UUID 查找用户
	FindByUuid(uuid string) (*model.UserInfo, error)
	// FindByTelephone 根据手机号查找用户
	FindByTelephone(telephone string) (*model.UserInfo, error)
	// FindAllExcept 查找除指定用户外的所有用户
	FindAllExcept(excludeUuid string) ([]model.UserInfo, error)
	// FindByUuids 批量根据 UUID 查找用户
	FindByUuids(uuids []string) ([]model.UserInfo, error)
	// CreateUser 创建新用户
	CreateUser(user *model.UserInfo) error
	// UpdateUserInfo 更新用户信息
	UpdateUserInfo(user *model.UserInfo) error
	// UpdateUserStatusByUuids [管理员] 批量更新用户状态（启用/禁用）
	UpdateUserStatusByUuids(uuids []string, status int8) error
	// UpdateUserIsAdminByUuids [管理员] 批量设置用户管理员权限
	UpdateUserIsAdminByUuids(uuids []string, isAdmin int8) error
	// SoftDeleteUserByUuids [管理员] 批量软删除用户
	SoftDeleteUserByUuids(uuids []string) error
	// FindAllPaged [管理员] 分页查询用户列表（支持关键词搜索和状态筛选）
	FindAllPaged(page, pageSize int, keyword string, status *int8) ([]model.UserInfo, int64, error)
}

// GroupRepository 群组数据访问接口
// 提供群组的增删改查操作
type GroupRepository interface {
	// FindByUuid 根据 UUID 查找群组
	FindByUuid(uuid string) (*model.GroupInfo, error)
	// FindByOwnerIdPaged 根据群主 ID 分页查找群组
	// ownerId: 群主用户UUID
	// page: 页码（从1开始）
	// pageSize: 每页数量
	// 返回: 群组列表、总数、错误
	FindByOwnerIdPaged(ownerId string, page, pageSize int) ([]model.GroupInfo, int64, error)
	// FindAll [管理员] 查找所有群组
	FindAll() ([]model.GroupInfo, error)
	// FindByUuids 批量根据 UUID 查找群组
	FindByUuids(uuids []string) ([]model.GroupInfo, error)
	// GetGroupList [管理员] 分页获取群组列表
	GetGroupList(page, pageSize int) ([]model.GroupInfo, int64, error)
	// CreateGroup 创建新群组
	CreateGroup(group *model.GroupInfo) error
	// Update 更新群组信息
	Update(group *model.GroupInfo) error

	// UpdateStatusByUuids [管理员] 批量更新群组状态
	UpdateStatusByUuids(uuids []string, status int8) error
	// IncrementMemberCount 增加群成员数量（+1）
	IncrementMemberCount(uuid string) error
	// DecrementMemberCountBy 减少群成员数量（指定数量）
	DecrementMemberCountBy(uuid string, count int) error
	// SoftDeleteByUuids [管理员] 批量软删除群组
	SoftDeleteByUuids(uuids []string) error
}

// ContactRepository 联系人数据访问接口
// 管理用户之间的好友关系
type ContactRepository interface {
	// FindByUserIdAndContactId 根据用户ID和联系人ID查找关系
	FindByUserIdAndContactId(userId, contactId string, contactType int8) (*model.Contact, error)
	// FindByUserIdAndType 根据用户ID和类型查找联系人（分页）
	FindByUserIdAndType(userId string, contactType int8, page, pageSize int) ([]model.Contact, int64, error)
	// FindUsersByContactId 根据联系人ID反向查找
	FindUsersByContactId(contactId string) ([]model.Contact, error)
	// CreateContact 创建联系人关系
	CreateContact(contact *model.Contact) error
	// IsFriend 判断两个用户是否互为好友
	IsFriend(userId1, userId2 string) (bool, error)
	// UpdateStatus 更新联系人状态（正常/拉黑等）
	UpdateStatus(userId, contactId string, contactType int8, status int8) error
	// SoftDelete 软删除联系人关系
	SoftDelete(userId, contactId string, contactType int8) error
	// SoftDeleteByUsers [管理员] 批量软删除指定用户的所有联系人（用于注销账号）
	SoftDeleteByUsers(userUuids []string) error
}

// SessionRepository 会话数据访问接口
// 管理聊天会话（用户之间或用户与群组之间）
type SessionRepository interface {
	// FindBySendIdAndReceiveId 根据发送者和接收者查找会话
	FindBySendIdAndReceiveId(sendId, receiveId string) (*model.Session, error)
	// FindBySendId 根据发送者ID查找所有会话
	FindBySendId(sendId string) ([]model.Session, error)
	// FindBySendIdPaged 根据发送者ID分页查找会话
	FindBySendIdPaged(sendId string, page, pageSize int) ([]model.Session, int64, error)
	// CreateSession 创建新会话
	CreateSession(session *model.Session) error
	// SoftDeleteByUuids 批量软删除会话
	SoftDeleteByUuids(uuids []string) error
	// SoftDeleteByUsers [管理员] 软删除指定用户的所有会话（用于注销账号）
	SoftDeleteByUsers(userUuids []string) error
	// UpdateByReceiveId 根据接收者ID更新会话字段
	UpdateByReceiveId(receiveId string, updates map[string]interface{}) error
	// UpdateLastMessage 更新会话的最后一条消息信息
	UpdateLastMessage(sendId, receiveId, content string, msgType int8, msgTime time.Time) error
}

// MessageRepository 消息数据访问接口
// 管理聊天消息的存取
type MessageRepository interface {
	// FindByUserIds 根据两个用户ID查找私聊消息
	FindByUserIds(userOneId, userTwoId string) ([]model.Message, error)
	// FindByUserIdsPaged 根据两个用户ID查找私聊消息（分页）
	FindByUserIdsPaged(userOneId, userTwoId string, page, pageSize int) ([]model.Message, error)
	// FindByGroupIdPaged 根据群组ID分页查找群聊消息
	FindByGroupIdPaged(groupId string, page, pageSize int) ([]model.Message, int64, error)
	// UpdateStatus 更新消息状态
	UpdateStatus(uuid string, status int8) error
	// Create 创建新消息
	Create(message *model.Message) error
	// FindLastMessageByUserIds 获取两人之间最后一条消息
	FindLastMessageByUserIds(userOneId, userTwoId string) (*model.Message, error)
	// FindLastMessageByGroupId 获取群组最后一条消息
	FindLastMessageByGroupId(groupId string) (*model.Message, error)
}

// ApplyRepository 联系人申请数据访问接口
// 管理好友申请和入群申请
type ApplyRepository interface {
	// FindByApplicantIdAndTargetId 根据申请人和目标查找申请
	FindByApplicantIdAndTargetId(applicantId, targetId string) (*model.Apply, error)
	// FindByTargetIdPendingPaged 分页查找目标用户的待处理申请
	// targetId: 目标用户/群组 UUID
	// page: 页码（从1开始）
	// pageSize: 每页数量
	// 返回: 申请列表、总数、错误
	FindByTargetIdPendingPaged(targetId string, page, pageSize int) ([]model.Apply, int64, error)
	// CreateApply 创建新申请
	CreateApply(apply *model.Apply) error
	// Update 更新申请信息
	Update(apply *model.Apply) error
	// SoftDelete 软删除申请
	SoftDelete(applicantId, targetId string) error
	// SoftDeleteByUsers [管理员] 批量软删除指定用户的所有申请（用于注销账号）
	SoftDeleteByUsers(userUuids []string) error
}

// GroupMemberRepository 群成员数据访问接口
// 管理群组成员关系
// GroupMemberWithUserInfo 定义在 model 包中
type GroupMemberRepository interface {
	// FindByGroupUuid 根据群组UUID查找所有成员
	FindByGroupUuid(groupUuid string) ([]model.GroupMember, error)
	// FindMembersWithUserInfoPaged 分页查找群成员（含用户详细信息）
	FindMembersWithUserInfoPaged(groupUuid string, page, pageSize int) ([]model.GroupMemberWithUserInfo, int64, error)
	// FindByGroupAndUser 根据群组UUID和用户UUID查找成员
	FindByGroupAndUser(groupUuid, userUuid string) (*model.GroupMember, error)
	// CreateGroupMember 添加群成员
	CreateGroupMember(member *model.GroupMember) error

	// DeleteByGroupUuid 删除群组所有成员
	DeleteByGroupUuid(groupUuid string) error
	// DeleteByUserUuids 批量删除指定用户
	DeleteByUserUuids(groupUuid string, userUuids []string) error
	// DeleteByGroupUuids [管理员] 批量删除多个群组的所有成员
	DeleteByGroupUuids(groupUuids []string) error
	// GetMemberIdsByGroupUuids [管理员] 获取多个群组的所有成员ID
	GetMemberIdsByGroupUuids(groupUuids []string) ([]string, error)
}
