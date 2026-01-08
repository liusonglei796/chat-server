// Package repository 定义数据访问层接口和聚合结构
// 采用 Repository 模式将数据访问逻辑与业务逻辑分离
// 所有 Repository 接口在此文件定义，具体实现在各自的文件中
package repository

import (
	"errors"

	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/errorx"

	"gorm.io/gorm"
)

// ==================== 错误包装辅助函数 ====================

// wrapDBError 包装数据库错误
// 根据错误类型返回不同的错误码：
//   - ErrRecordNotFound -> CodeNotFound
//   - 其他错误 -> CodeDBError
//
// err: 原始错误
// msg: 错误描述
// 返回: 包装后的错误
func wrapDBError(err error, msg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorx.Wrap(err, errorx.CodeNotFound, msg)
	}
	return errorx.Wrap(err, errorx.CodeDBError, msg)
}

// wrapDBErrorf 包装数据库错误（支持格式化消息）
// 功能同 wrapDBError，但支持 fmt.Sprintf 风格的格式化
func wrapDBErrorf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorx.Wrapf(err, errorx.CodeNotFound, format, args...)
	}
	return errorx.Wrapf(err, errorx.CodeDBError, format, args...)
}

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
	// Create 创建新用户
	Create(user *model.UserInfo) error
	// UpdateUserInfo 更新用户信息
	UpdateUserInfo(user *model.UserInfo) error
	// UpdateUserStatusByUuids 批量更新用户状态（启用/禁用）
	UpdateUserStatusByUuids(uuids []string, status int8) error
	// UpdateUserIsAdminByUuids 批量设置用户管理员权限
	UpdateUserIsAdminByUuids(uuids []string, isAdmin int8) error
	// SoftDeleteUserByUuids 批量软删除用户
	SoftDeleteUserByUuids(uuids []string) error
}

// GroupRepository 群组数据访问接口
// 提供群组的增删改查操作
type GroupRepository interface {
	// FindByUuid 根据 UUID 查找群组
	FindByUuid(uuid string) (*model.GroupInfo, error)
	// FindByOwnerId 根据群主 ID 查找群组
	FindByOwnerId(ownerId string) ([]model.GroupInfo, error)
	// FindAll 查找所有群组
	FindAll() ([]model.GroupInfo, error)
	// FindByUuids 批量根据 UUID 查找群组
	FindByUuids(uuids []string) ([]model.GroupInfo, error)
	// GetList 分页获取群组列表
	GetGroupList(page, pageSize int) ([]model.GroupInfo, int64, error)
	// Create 创建新群组
	Create(group *model.GroupInfo) error
	// Update 更新群组信息
	Update(group *model.GroupInfo) error

	// UpdateStatusByUuids 批量更新群组状态
	UpdateStatusByUuids(uuids []string, status int8) error
	// IncrementMemberCount 增加群成员数量（+1）
	IncrementMemberCount(uuid string) error
	// DecrementMemberCountBy 减少群成员数量（指定数量）
	DecrementMemberCountBy(uuid string, count int) error
	// SoftDeleteByUuids 批量软删除群组
	SoftDeleteByUuids(uuids []string) error
}

// ContactRepository 联系人数据访问接口
// 管理用户之间的好友关系
type ContactRepository interface {
	// FindByUserIdAndContactId 根据用户ID和联系人ID查找关系
	FindByUserIdAndContactId(userId, contactId string) (*model.Contact, error)
	// FindByUserIdAndType 根据用户ID和联系人类型查找
	FindByUserIdAndType(userId string, contactType int8) ([]model.Contact, error)
	// FindUsersByContactId 根据联系人ID反向查找
	FindUsersByContactId(contactId string) ([]model.Contact, error)
	// Create 创建联系人关系
	Create(contact *model.Contact) error
	// UpdateStatus 更新联系人状态（正常/拉黑等）
	UpdateStatus(userId, contactId string, status int8) error
	// SoftDelete 软删除联系人关系
	SoftDelete(userId, contactId string) error
	// SoftDeleteByUsers 批量软删除指定用户的所有联系人
	SoftDeleteByUsers(userUuids []string) error
}

// SessionRepository 会话数据访问接口
// 管理聊天会话（用户之间或用户与群组之间）
type SessionRepository interface {
	// FindBySendIdAndReceiveId 根据发送者和接收者查找会话
	FindBySendIdAndReceiveId(sendId, receiveId string) (*model.Session, error)
	// FindBySendId 根据发送者ID查找所有会话
	FindBySendId(sendId string) ([]model.Session, error)
	// Create 创建新会话
	Create(session *model.Session) error
	// SoftDeleteByUuids 批量软删除会话
	SoftDeleteByUuids(uuids []string) error
	// SoftDeleteByUsers 软删除指定用户的所有会话
	SoftDeleteByUsers(userUuids []string) error
	// UpdateByReceiveId 根据接收者ID更新会话字段
	UpdateByReceiveId(receiveId string, updates map[string]interface{}) error
}

// MessageRepository 消息数据访问接口
// 管理聊天消息的存取
type MessageRepository interface {
	// FindByUserIds 根据两个用户ID查找私聊消息
	FindByUserIds(userOneId, userTwoId string) ([]model.Message, error)
	// FindByGroupId 根据群组ID查找群聊消息
	FindByGroupId(groupId string) ([]model.Message, error)
}

// ApplyRepository 联系人申请数据访问接口
// 管理好友申请和入群申请
type ApplyRepository interface {
	// FindByApplicantIdAndTargetId 根据申请人和目标查找申请
	FindByApplicantIdAndTargetId(applicantId, targetId string) (*model.Apply, error)
	// FindByTargetIdPending 查找目标用户的待处理申请
	FindByTargetIdPending(targetId string) ([]model.Apply, error)
	// Create 创建新申请
	Create(apply *model.Apply) error
	// Update 更新申请信息
	Update(apply *model.Apply) error
	// SoftDelete 软删除申请
	SoftDelete(applicantId, targetId string) error
	// SoftDeleteByUsers 批量软删除指定用户的所有申请
	SoftDeleteByUsers(userUuids []string) error
}

// ==================== 复合结构 ====================

// GroupMemberWithUserInfo 群成员详细信息（含用户资料）
// 用于群成员列表展示，包含用户的基本信息
type GroupMemberWithUserInfo struct {
	UserId   string `json:"userId"`   // 用户 UUID
	Nickname string `json:"nickname"` // 用户昵称
	Avatar   string `json:"avatar"`   // 用户头像
}

// GroupMemberRepository 群成员数据访问接口
// 管理群组成员关系
type GroupMemberRepository interface {

	// FindMembersWithUserInfo 查找群成员（含用户详细信息）
	FindMembersWithUserInfo(groupUuid string) ([]GroupMemberWithUserInfo, error)
	// Create 添加群成员
	Create(member *model.GroupMember) error

	// DeleteByGroupUuid 删除群组所有成员
	DeleteByGroupUuid(groupUuid string) error
	// DeleteByUserUuids 批量删除指定用户
	DeleteByUserUuids(groupUuid string, userUuids []string) error
	// DeleteByGroupUuids 批量删除多个群组的所有成员
	DeleteByGroupUuids(groupUuids []string) error
	// GetMemberIdsByGroupUuids 获取多个群组的所有成员ID
	GetMemberIdsByGroupUuids(groupUuids []string) ([]string, error)
}

// ==================== Repository 聚合 ====================

// Repositories 聚合所有 Repository 实例
// 作为依赖注入的入口，Service 层通过此结构访问数据层
type Repositories struct {
	db          *gorm.DB              // GORM 数据库实例
	User        UserRepository        // 用户 Repository
	Group       GroupRepository       // 群组 Repository
	Contact     ContactRepository     // 联系人 Repository
	Session     SessionRepository     // 会话 Repository
	Message     MessageRepository     // 消息 Repository
	Apply       ApplyRepository       // 申请 Repository
	GroupMember GroupMemberRepository // 群成员 Repository
}

// NewRepositories 创建所有 Repository 实例
// 接收 GORM 数据库实例，初始化并返回 Repositories 聚合
// db: GORM 数据库实例
// 返回: Repositories 聚合指针
func NewRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		db:          db,
		User:        NewUserRepository(db),
		Group:       NewGroupRepository(db),
		Contact:     NewContactRepository(db),
		Session:     NewSessionRepository(db),
		Message:     NewMessageRepository(db),
		Apply:       NewApplyRepository(db),
		GroupMember: NewGroupMemberRepository(db),
	}
}

// Transaction 在数据库事务中执行函数
// 事务内的所有操作要么全部成功，要么全部回滚
// fn: 事务执行函数，接收事务内的 Repositories 实例
// 返回: 操作错误（如有错误会自动回滚）
func (r *Repositories) Transaction(fn func(txRepos *Repositories) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 使用事务 db 创建新的 Repositories 实例
		return fn(NewRepositories(tx))
	})
}
