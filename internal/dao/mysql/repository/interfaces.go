package repository

import (
	"errors"

	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/errorx"

	"gorm.io/gorm"
)

// wrapDBError 包装数据库错误
// 如果是 ErrRecordNotFound，返回 CodeNotFound；否则返回 CodeDBError
func wrapDBError(err error, msg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorx.Wrap(err, errorx.CodeNotFound, msg)
	}
	return errorx.Wrap(err, errorx.CodeDBError, msg)
}

// wrapDBErrorf 包装数据库错误（格式化消息）
func wrapDBErrorf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorx.Wrapf(err, errorx.CodeNotFound, format, args...)
	}
	return errorx.Wrapf(err, errorx.CodeDBError, format, args...)
}

// UserRepository 用户数据访问接口
type UserRepository interface {
	FindByUuid(uuid string) (*model.UserInfo, error)
	FindByTelephone(telephone string) (*model.UserInfo, error)
	FindAllExcept(excludeUuid string) ([]model.UserInfo, error)
	FindByUuids(uuids []string) ([]model.UserInfo, error)
	Create(user *model.UserInfo) error
	UpdateUserInfo(user *model.UserInfo) error
	UpdateUserStatusByUuids(uuids []string, status int8) error
	UpdateUserIsAdminByUuids(uuids []string, isAdmin int8) error
	SoftDeleteUserByUuids(uuids []string) error
}

// GroupRepository 群组数据访问接口
type GroupRepository interface {
	FindByUuid(uuid string) (*model.GroupInfo, error)
	FindByOwnerId(ownerId string) ([]model.GroupInfo, error)
	FindAll() ([]model.GroupInfo, error)
	FindByUuids(uuids []string) ([]model.GroupInfo, error)
	GetList(page, pageSize int) ([]model.GroupInfo, int64, error)
	Create(group *model.GroupInfo) error
	Update(group *model.GroupInfo) error
	UpdateStatus(uuid string, status int8) error
	UpdateStatusByUuids(uuids []string, status int8) error
	IncrementMemberCount(uuid string) error
	DecrementMemberCount(uuid string) error
	DecrementMemberCountBy(uuid string, count int) error
	SoftDeleteByUuids(uuids []string) error
}

// ContactRepository 联系人数据访问接口
type ContactRepository interface {
	FindByUserIdAndContactId(userId, contactId string) (*model.UserContact, error)
	FindByUserId(userId string) ([]model.UserContact, error)
	FindByUserIdAndType(userId string, contactType int8) ([]model.UserContact, error)
	FindByContactId(contactId string) ([]model.UserContact, error)
	Create(contact *model.UserContact) error
	Update(contact *model.UserContact) error
	UpdateStatus(userId, contactId string, status int8) error
	SoftDelete(userId, contactId string) error
	SoftDeleteByUsers(userUuids []string) error
}

// SessionRepository 会话数据访问接口
type SessionRepository interface {
	FindByUuid(uuid string) (*model.Session, error)
	FindBySendIdAndReceiveId(sendId, receiveId string) (*model.Session, error)
	FindBySendId(sendId string) ([]model.Session, error)
	FindByReceiveId(receiveId string) ([]model.Session, error)
	Create(session *model.Session) error
	Update(session *model.Session) error
	SoftDeleteByUuids(uuids []string) error
	SoftDeleteByUsers(userUuids []string) error
	UpdateByReceiveId(receiveId string, updates map[string]interface{}) error
}

// MessageRepository 消息数据访问接口
type MessageRepository interface {
	FindBySessionId(sessionId string) ([]model.Message, error)
	FindByUserIds(userOneId, userTwoId string) ([]model.Message, error)
	FindByGroupId(groupId string) ([]model.Message, error)
	Create(message *model.Message) error
}

// ContactApplyRepository 联系人申请数据访问接口
type ContactApplyRepository interface {
	FindByUuid(uuid string) (*model.ContactApply, error)
	FindByApplicantIdAndTargetId(applicantId, targetId string) (*model.ContactApply, error)
	FindByTargetIdPending(targetId string) ([]model.ContactApply, error)
	FindByTargetIdAndType(targetId string, contactType int8) ([]model.ContactApply, error)
	Create(apply *model.ContactApply) error
	Update(apply *model.ContactApply) error
	UpdateStatus(uuid string, status int8) error
	SoftDelete(applicantId, targetId string) error
	SoftDeleteByUsers(userUuids []string) error
}

// GroupMemberWithUserInfo 群成员详细信息（含用户资料）
type GroupMemberWithUserInfo struct {
	UserId   string `json:"userId"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// GroupMemberRepository 群成员数据访问接口
type GroupMemberRepository interface {
	FindByGroupUuid(groupUuid string) ([]model.GroupMember, error)
	FindByUserUuid(userUuid string) ([]model.GroupMember, error)
	FindByGroupAndUser(groupUuid, userUuid string) (*model.GroupMember, error)
	FindMembersWithUserInfo(groupUuid string) ([]GroupMemberWithUserInfo, error)
	Create(member *model.GroupMember) error
	Delete(groupUuid, userUuid string) error
	DeleteByGroupUuid(groupUuid string) error
	DeleteByUserUuids(groupUuid string, userUuids []string) error
	DeleteByGroupUuids(groupUuids []string) error
	GetMemberIdsByGroupUuids(groupUuids []string) ([]string, error)
}

// Repositories 聚合所有 Repository
type Repositories struct {
	db           *gorm.DB
	User         UserRepository
	Group        GroupRepository
	Contact      ContactRepository
	Session      SessionRepository
	Message      MessageRepository
	ContactApply ContactApplyRepository
	GroupMember  GroupMemberRepository
}

// NewRepositories 创建所有 Repository 实例
func NewRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		db:           db,
		User:         NewUserRepository(db),
		Group:        NewGroupRepository(db),
		Contact:      NewContactRepository(db),
		Session:      NewSessionRepository(db),
		Message:      NewMessageRepository(db),
		ContactApply: NewContactApplyRepository(db),
		GroupMember:  NewGroupMemberRepository(db),
	}
}

// Transaction 在事务中执行函数
func (r *Repositories) Transaction(fn func(txRepos *Repositories) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(NewRepositories(tx))
	})
}
