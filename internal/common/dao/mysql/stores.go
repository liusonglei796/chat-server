// Package mysql 提供 Store 层聚合与构造
package mysql

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"kama_chat_server/internal/common/dao/mysql/apply"
	"kama_chat_server/internal/common/dao/mysql/friendship"
	"kama_chat_server/internal/common/dao/mysql/group"
	"kama_chat_server/internal/common/dao/mysql/member"
	"kama_chat_server/internal/common/dao/mysql/message"
	"kama_chat_server/internal/common/dao/mysql/outbox"
	"kama_chat_server/internal/common/dao/mysql/session"
	"kama_chat_server/internal/common/dao/mysql/user"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/infrastructure/snowflake"
	"kama_chat_server/internal/common/model"
)

// Stores 聚合所有 Store 实例
// 作为依赖注入的入口，Service 层通过此结构访问数据层
type Stores struct {
	db          *gorm.DB                   // GORM 数据库实例
	User        store.UserStore  // 用户 Store
	Group       store.GroupStore // 群组 Store
	Friendship  store.FriendshipStore
	Session     store.SessionStore
	Message     store.MessageStore
	Apply       store.ApplyStore
	GroupMember store.GroupMemberStore
	Outbox      store.OutboxStore
}

// NewStores 创建所有 Store 实例
func NewStores(db *gorm.DB) *Stores {
	return &Stores{
		db:          db,
		User:        user.NewUserStore(db),
		Group:       group.NewGroupStore(db),
		Friendship:  friendship.NewFriendshipStore(db),
		Session:     session.NewSessionStore(db),
		Message:     message.NewMessageStore(db),
		Apply:       apply.NewApplyStore(db),
		GroupMember: member.NewGroupMemberStore(db),
		Outbox:      outbox.NewOutboxStore(db),
	}
}

// 确保 Stores 实现了事务执行能力（各服务 UoW 接口的组合部分）
var _ store.TxExecutor = (*Stores)(nil)

// UserStore 返回用户 Store
func (r *Stores) UserStore() store.UserStore { return r.User }

// GroupStore 返回群组 Store
func (r *Stores) GroupStore() store.GroupStore { return r.Group }

// FriendshipStore 返回好友关系 Store
func (r *Stores) FriendshipStore() store.FriendshipStore { return r.Friendship }

// SessionStore 返回会话 Store
func (r *Stores) SessionStore() store.SessionStore { return r.Session }

// MessageStore 返回消息 Store
func (r *Stores) MessageStore() store.MessageStore { return r.Message }

// ApplyStore 返回申请 Store
func (r *Stores) ApplyStore() store.ApplyStore { return r.Apply }

// GroupMemberStore 返回群成员 Store
func (r *Stores) GroupMemberStore() store.GroupMemberStore { return r.GroupMember }

// OutboxStore 返回发件箱 Store
func (r *Stores) OutboxStore() store.OutboxStore { return r.Outbox }

// RecordEvent 在事务内写一条领域事件到 outbox 表
// 事件类型与载荷由调用方给出，UUID/状态/时间戳在此统一构造
func (r *Stores) RecordEvent(ctx context.Context, eventType string, payload []byte) error {
	o := model.Outbox{
		Uuid:      fmt.Sprintf("O%s", snowflake.GenerateIDString()),
		EventType: eventType,
		Payload:   string(payload),
		Status:    0,
		CreatedAt: time.Now(),
	}
	return r.Outbox.Create(ctx, &o)
}

// WithTx 在数据库事务中执行函数
// 回调参数 tx 是绑定了事务连接的新 Stores（具体实现类型，由调用方断言为服务子接口），
// 回调内所有 Store 操作共享同一事务：返回 nil 提交，返回 error 回滚
func (r *Stores) WithTx(fn func(tx any) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(NewStores(tx))
	})
}
