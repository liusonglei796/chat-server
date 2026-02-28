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
	"kama_chat_server/internal/service/mysqlinterface"
)

// Repositories 聚合所有 Repository 实例
// 作为依赖注入的入口，Service 层通过此结构访问数据层
type Repositories struct {
	db          *gorm.DB                       // GORM 数据库实例
	User        mysqlinterface.UserRepository  // 用户 Repository
	Group       mysqlinterface.GroupRepository // 群组 Repository
	Friendship  mysqlinterface.FriendshipRepository
	Session     mysqlinterface.SessionRepository
	Message     mysqlinterface.MessageRepository
	Apply       mysqlinterface.ApplyRepository
	GroupMember mysqlinterface.GroupMemberRepository
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

// Transaction 在数据库事务中执行函数
func (r *Repositories) Transaction(fn func(txRepos *Repositories) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(NewRepositories(tx))
	})
}
