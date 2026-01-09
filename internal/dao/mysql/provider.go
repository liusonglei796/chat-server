// Package mysql 提供 Repository 层聚合与构造
package mysql

import (
	"gorm.io/gorm"

	"kama_chat_server/internal/dao/mysql/apply"
	"kama_chat_server/internal/dao/mysql/contact"
	"kama_chat_server/internal/dao/mysql/group"
	"kama_chat_server/internal/dao/mysql/member"
	"kama_chat_server/internal/dao/mysql/message"
	"kama_chat_server/internal/dao/mysql/session"
	"kama_chat_server/internal/dao/mysql/user"
)

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
		User:        user.NewUserRepository(db),
		Group:       group.NewGroupRepository(db),
		Contact:     contact.NewContactRepository(db),
		Session:     session.NewSessionRepository(db),
		Message:     message.NewMessageRepository(db),
		Apply:       apply.NewApplyRepository(db),
		GroupMember: member.NewGroupMemberRepository(db),
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
