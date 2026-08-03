// Package mysql 提供 Repository 层聚合与构造
package mysql

import (
	"gorm.io/gorm"

	"kama_chat_server/internal/common/dao/mysql/apply"
	"kama_chat_server/internal/common/dao/mysql/friendship"
	"kama_chat_server/internal/common/dao/mysql/group"
	"kama_chat_server/internal/common/dao/mysql/member"
	"kama_chat_server/internal/common/dao/mysql/message"
	"kama_chat_server/internal/common/dao/mysql/outbox"
	"kama_chat_server/internal/common/dao/mysql/session"
	"kama_chat_server/internal/common/dao/mysql/user"
	"kama_chat_server/internal/common/domain/repository"
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
	Outbox      repository.OutboxRepository
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
		Outbox:      outbox.NewOutboxRepository(db),
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

// OutboxRepo 返回发件箱 Repository
func (r *Repositories) OutboxRepo() repository.OutboxRepository { return r.Outbox }

// WithTx 在数据库事务中执行函数
// 回调参数 tx 是绑定了事务连接的新 Repositories，
// 回调内所有 Repository 操作共享同一事务：返回 nil 提交，返回 error 回滚
func (r *Repositories) WithTx(fn func(uow repository.UnitOfWork) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(NewRepositories(tx))
	})
}
