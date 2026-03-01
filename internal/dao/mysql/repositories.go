// Package mysql 提供 Repository 层聚合与构造
package mysql

import (
	"gorm.io/gorm"

	"kama_chat_server/internal/dao/mysql/apply"
	"kama_chat_server/internal/dao/mysql/friendship"
	"kama_chat_server/internal/dao/mysql/group"
	"kama_chat_server/internal/dao/mysql/member"
	"kama_chat_server/internal/dao/mysql/message"
	"kama_chat_server/internal/dao/mysql/session"
	"kama_chat_server/internal/dao/mysql/user"
	"kama_chat_server/internal/domain/repository"
)

// Repositories 聚合所有 Repository 实例
// 作为依赖注入的入口，Service 层通过此结构访问数据层
type Repositories struct {
	db          *gorm.DB                   // GORM 数据库实例
	User        repository.UserRepository  // 用户 Repository
	Group       repository.GroupRepository // 群组 Repository
	Friendship  repository.FriendshipRepository
	Session     repository.SessionRepository
	Message     repository.MessageRepository
	Apply       repository.ApplyRepository
	GroupMember repository.GroupMemberRepository
}

// NewRepositories 创建所有 Repository 实例
func NewRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		db:          db,
		User:        user.NewUserRepository(db),
		Group:       group.NewGroupRepository(db),
		Friendship:  friendship.NewFriendshipRepository(db),
		Session:     session.NewSessionRepository(db),
		Message:     message.NewMessageRepository(db),
		Apply:       apply.NewApplyRepository(db),
		GroupMember: member.NewGroupMemberRepository(db),
	}
}

// 确保 Repositories 实现了 UnitOfWork 接口
var _ repository.UnitOfWork = (*Repositories)(nil)

// UserRepo 返回用户 Repository
func (r *Repositories) UserRepo() repository.UserRepository { return r.User }

// GroupRepo 返回群组 Repository
func (r *Repositories) GroupRepo() repository.GroupRepository { return r.Group }

// FriendshipRepo 返回好友关系 Repository
func (r *Repositories) FriendshipRepo() repository.FriendshipRepository { return r.Friendship }

// SessionRepo 返回会话 Repository
func (r *Repositories) SessionRepo() repository.SessionRepository { return r.Session }

// MessageRepo 返回消息 Repository
func (r *Repositories) MessageRepo() repository.MessageRepository { return r.Message }

// ApplyRepo 返回申请 Repository
func (r *Repositories) ApplyRepo() repository.ApplyRepository { return r.Apply }

// GroupMemberRepo 返回群成员 Repository
func (r *Repositories) GroupMemberRepo() repository.GroupMemberRepository { return r.GroupMember }

// Transaction 在数据库事务中执行函数
func (r *Repositories) Transaction(fn func(uow repository.UnitOfWork) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(NewRepositories(tx))
	})
}
