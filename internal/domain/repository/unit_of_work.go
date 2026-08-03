package repository

// UnitOfWork 工作单元接口
// 提供事务支持和 Repository 访问器
// Service 层通过此接口访问多个 Repository 并在事务中协调操作
type UnitOfWork interface {
	UserRepo() UserRepository
	GroupRepo() GroupRepository
	FriendshipRepo() FriendshipRepository
	SessionRepo() SessionRepository
	MessageRepo() MessageRepository
	ApplyRepo() ApplyRepository
	GroupMemberRepo() GroupMemberRepository
	OutboxRepo() OutboxRepository

	// WithTx 在数据库事务中执行函数
	// 回调参数 tx 是绑定了事务连接的新 UnitOfWork 实例，
	// 回调内所有 Repository 操作都运行在同一事务中；
	// 回调返回 nil 则提交，返回 error 则回滚。
	WithTx(fn func(tx UnitOfWork) error) error
}
