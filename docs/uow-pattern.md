# Unit of Work 模式与回调机制解析

> 本文档解释本项目（chat-server）中 Unit of Work（工作单元）模式的设计与实现，
> 重点说明"回调（callback）"这一核心机制是如何在代码中体现的。

> **⚠️ 现状说明（2026-08 重构后）**
> 本文档描述的"全家桶式共享 `UnitOfWork`"设计已废弃。当前实现是**每个服务一个精简 UoW 接口**：
> 每个服务只声明自己拥有的 Store 访问器（如 `groupUoW` 仅含 `GroupStore()`/`GroupMemberStore()`），
> 从根上杜绝跨库访问。共享部分只剩一个纯事务契约
> `store.TxExecutor`（`WithTx(fn func(tx any) error) error`）+ 泛型辅助 `store.WithTx[T]`。
> 下文概念（回调、事务连接到达业务代码、两层嵌套）仍然成立，但接口签名以实际代码为准。

---

## 1. 什么是 Unit of Work（工作单元）

**工作单元**是一种设计模式：把"多个数据操作"绑定到**同一个事务**中，统一提交或统一回滚，
保证一组操作要么全部成功、要么全部失败。

### 为什么聊天系统需要它

很多业务操作要同时修改多张表，例如**建群**：

```go
// 三步操作必须原子化：
// 1. 插入群记录          (group_info)
// 2. 插入群主成员记录    (group_member)
// 3. 插入会话记录        (session)
```

如果不用事务，可能出现"群建好了但群主不在群里"的脏数据。
工作单元把这三步包进一个事务，任何一步失败就整体回滚。

### 与"Repository"的关系

- **Repository（仓库）**：单张表的数据访问（增删改查）
- **Unit of Work（工作单元）**：协调多个 Repository，让它们共享一个事务

---

## 2. 本项目中的完整链路

### 2.1 接口定义（domain 层）

`internal/domain/store/transaction.go`

```go
type UnitOfWork interface {
	UserStore() UserStore
	GroupStore() GroupStore
	FriendshipStore() FriendshipStore
	SessionStore() SessionStore
	MessageStore() MessageStore
	ApplyStore() ApplyStore
	GroupMemberStore() GroupMemberStore

	// WithTx 在数据库事务中执行函数
	// 回调参数 tx 是绑定了事务连接的新 UnitOfWork 实例，
	// 回调内所有 Repository 操作都运行在同一事务中；
	// 回调返回 nil 则提交，返回 error 则回滚。
	WithTx(fn func(tx UnitOfWork) error) error
}
```

要点：

- 接口定义在 **domain 层**，Service 层只依赖这个接口，不接触任何数据库细节
- `WithTx` 是核心：接收一个"回调函数"，在事务中执行它
- **自引用**：回调参数 `tx UnitOfWork` 的类型就是接口本身

### 2.2 实现（infrastructure 层）

`internal/dao/mysql/stores.go`

```go
type Stores struct {
	db          *gorm.DB
	User        store.UserStore
	Group       store.GroupStore
	Friendship  store.FriendshipStore
	Session     store.SessionStore
	Message     store.MessageStore
	Apply       store.ApplyStore
	GroupMember store.GroupMemberStore
}

// 编译期断言：保证 Stores 确实实现了 UnitOfWork 接口
var _ store.TxExecutor = (*Stores)(nil)

func (r *Stores) WithTx(fn func(tx any) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(NewStores(tx))
	})
}
```

要点：

- `Stores` 持有 `*gorm.DB` 和 7 个 Repository 实例
- 事务本身**没有自己实现**——直接使用 GORM 的 `db.Transaction`（BEGIN/COMMIT/ROLLBACK 全在 gorm 内部）
- 项目的唯一贡献是 `NewStores(tx)`：**用事务连接重建一套 Repository**

### 2.3 注入（组合根）

`internal/service/services.go`

```go
func NewServices(tx store.TxExecutor, cacheService store.AsyncCacheService, ...) *Services {
	// 事务型 Service：注入整个 UnitOfWork 接口
	userSvc := user.NewUserService(uow, cacheService)
	groupSvc := group.NewGroupService(uow, cacheService)
	// ...

	// 非事务型 Service：只注入单个 Repository
	sessionSvc := session.NewSessionService(uow.SessionStore(), ...)
}
```

### 2.4 使用（service 层）

`internal/service/group/service.go` — 建群：

```go
err := g.uow.WithTx(func(tx store.TxExecutor) error {
	tx.GroupStore().CreateGroup(ctx, &group)             // 1. 插入群
	tx.GroupMemberStore().CreateGroupMember(ctx, &member) // 2. 插入群主成员
	tx.SessionStore().CreateSession(ctx, &session)        // 3. 插入会话
	return nil
})
```

全项目共 **15 处** `WithTx` 调用，分布在 6 个 Service 文件。

---

## 3. 核心机制：事务连接是怎么"到达"业务代码的

### 3.1 关键事实：Repository 是无状态的"壳"

```go
type userStore struct {
	db *gorm.DB   // 唯一字段：一个数据库连接
}

func (r *userStore) FindByUuid(ctx context.Context, uuid string) (*model.UserInfo, error) {
	return r.db.WithContext(ctx).First(&user, "uuid = ?", uuid).Error  // SQL 走 r.db
}
```

Repository 没有任何业务状态，所有 SQL 都通过 `r.db` 执行。
**谁持有 db，谁的 SQL 就走哪条连接。**

### 3.2 绑定的动作：`NewStores(tx)`

GORM 的 `db.Transaction` 内部会：

```go
tx := db.Begin()        // 从连接池取出专用连接，开启事务，包成新的 *gorm.DB
fc(tx)                  // 把事务连接 tx 交给回调
tx.Commit() / Rollback() // 根据回调返回值决定
```

`WithTx` 拿到事务连接 `tx` 后，用它重建整套 Repository：

```go
return fn(NewStores(tx))
//          └─────────────────┐
//   &Stores{
//       User:       &userStore{db: tx},     ← db 指向事务连接
//       Group:      &groupStore{db: tx},    ← 全部绑定到同一事务
//       ...
//   }
```

**为什么要重建而不是复用 `r`？**

因为 `r` 里的 Repo 绑定的是**连接池**。如果直接 `fn(r)`，回调里的 SQL 会走连接池，
不受 `Begin/Commit/Rollback` 控制，事务形同虚设。

> 由于 Repo 是无状态的壳，重建 7 个 struct 的代价可忽略（纳秒级，对比毫秒级的 SQL 往返）。

### 3.3 结构体如何满足接口

`NewStores(tx)` 返回的是 `*Stores`（结构体指针），不是 `UnitOfWork`。
但 `*Stores` 拥有接口要求的全部方法，Go 的**隐式接口满足**让它自动成为 `UnitOfWork`：

```go
// 方法全是指针接收者 → 只有 *Stores 实现接口
func (r *Stores) WithTx(...) error { ... }

var _ store.TxExecutor = (*Stores)(nil)  // ✅ 编译通过 = 实现了
var _ store.TxExecutor = (Stores)(nil)   // ❌ 值类型不实现
```

---

## 4. 回调机制：体现在哪里

### 4.1 回调的定义

> **函数是你写的，但调用权在别人手里。** 你交出函数，由框架在合适的时机主动调用它
> ——这就是回调（callback），本质是**控制反转（IoC）**。

### 4.2 回调的三个体现点

| 体现点 | 位置 | 作用 |
|---|---|---|
| ① 形参声明 | 接口 `WithTx(fn func(tx UnitOfWork) error) error` | 参数类型本身就是函数类型，定义契约 |
| ② 调用处注册 | service 里的匿名函数字面量 | 你"交"出函数，此刻不执行 |
| ③ 内部调用 | `fn(NewStores(tx))` | 回调被真正调用的那一行 |

### 4.3 两层嵌套回调

这个设计其实是**两层回调套娃**——你的回调包在 GORM 的回调里：

```go
// 你写的代码（service 层）
err := uow.WithTx(func(tx UnitOfWork) error {     // 回调 A：你定义，你不管何时跑
	tx.UserStore().Create(user)
	return nil
})

// WithTx 实现（repositories.go）
func (r *Stores) WithTx(fn func(tx any) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {   // 回调 B：也是匿名函数
		return fn(NewStores(tx))                   // ← 回调 A 在这里被调用
	})
}

// gorm 内部（简化）
func (db *DB) Transaction(fc func(tx *DB) error) error {
	tx := db.Begin()
	if err := fc(tx); err == nil {
		return tx.Commit().Error
	}
	tx.Rollback()
	return tx.Error
}
```

### 4.4 完整执行时序

```
你写代码    ① uow.WithTx(A)             → A 被"递"给 WithTx，函数体没跑
WithTx 内   ② r.db.Transaction(B)       → B 被"递"给 gorm，也没跑
gorm 内     ③ tx := db.Begin()          → 事务建立
            ④ B(tx)                     → gorm 先调用自己的回调 B
            ⑤ B 内: A(NewStores(tx)) → 再调用你的回调 A ← 业务代码此刻才真正执行
            ⑥ A 返回 nil → Commit；返回 err → Rollback
```

**关键认知**：你的业务代码执行时机（⑤）比注册它的那行（①）晚了一整个事务建立过程——这就是**延迟执行**。

### 4.5 对比"普通调用"和"回调"

```go
// 普通调用：你直接调它
result := add(1, 2)          // 调用时机由你控制

// 回调：你被它调用
uow.WithTx(func(tx UnitOfWork) error { ... })   // 调用时机由 WithTx/gorm 控制
```

---

## 5. 自引用设计与嵌套事务

> **⚠️ 现状**：自引用是"共享全家桶 UoW"时代的特性。当前 `WithTx(fn func(tx any) error)` 的
> 回调参数是 `any`，由泛型辅助 `store.WithTx[T]` 断言为具体服务的 UoW 接口，**不再自引用**。
> 下面的示例仅用于说明嵌套事务与 savepoint 机制。

因为回调参数 `tx` 的类型就是 `UnitOfWork` 接口本身，函数体内可以**再次调用 WithTx**：

```go
uow.WithTx(func(tx UnitOfWork) error {
	tx.UserStore().Create(user)                     // 外层事务
	return tx.WithTx(func(tx2 UnitOfWork) error {  // 嵌套事务
		tx2.MessageStore().Create(msg)
		return nil
	})
})
```

GORM 对嵌套事务有原生支持（**savepoint** 机制）：内层失败只回滚内层，外层仍可提交。

---

## 6. 常见问题

### Q1: 项目是自己实现事务还是用 GORM 的？

**用 GORM 的。** 全项目搜不到任何手动 `Begin()/Commit()/Rollback()`。
`WithTx` 只是包装——它的唯一职责是 `NewStores(tx)`（让事务连接到达业务代码），
事务本身的开启、提交、回滚、panic 兜底全是 `(*gorm.DB).Transaction` 在做。

### Q2: 为什么不能去掉包装直接用 `db.Transaction`？

可以，但代价是：15 处调用点各自重建 Repo + Service 层依赖 `mysql` 包（破坏分层）+ 无法 mock 单测。
包装只有 3 行，却换来"调用点零成本 + 依赖方向正确 + 可测试"。

### Q3: 包装了不还是每次事务都重建 Repo 吗？

是的，重建**每次事务都发生**。但包装省的不是重建本身，而是**每个调用点的重建代码和分层破坏**
——15 处调用点不需要知道 `NewStores(tx)` 的存在。重建 7 个 struct 的成本可忽略。

### Q4: 不用 UOW 有哪些替代方案？

| 方案 | 说明 | 代价 |
|---|---|---|
| Service 直接用 gorm | 事务内手动重建 Repo | 15 处重复 + 分层破坏 |
| 事务连接塞进 context | Repo 从 context 取 tx | 隐式语义，难排查 |
| 补偿/最终一致 | 容忍脏数据异步修复 | 复杂度远超事务 |
| **UOW（本项目）** | 集中绑定 + 接口隔离 | —— |

### Q5: 为什么接口方法没有函数体？

接口只定义**契约**（能干什么），不定义**实现**（怎么干）。
函数体在实现者 `Stores.WithTx` 中（`internal/dao/mysql/stores.go`）。
调用 `uow.WithTx(...)` 时，Go 根据接口中存储的真实类型（`*Stores`）**动态派发**到实现。

---

## 7. 相关代码文件索引

| 文件 | 角色 |
|---|---|
| `internal/domain/store/transaction.go` | 接口定义（契约） |
| `internal/dao/mysql/stores.go` | 实现（函数体所在） |
| `internal/service/services.go` | 注入（组合根） |
| `internal/service/group/service.go` | 使用示例（建群，15 处之一） |
| `internal/dao/mysql/*/xxx_repository.go` | 各 Repository 实现（持 db 的壳） |

---

## 8. 一句话总结

> **UOW = 用接口把"多个 Repository 共享一个事务"这件事抽象出来；
> 回调 = 把"你的业务代码"交给事务框架，由它在事务就绪后调用；
> 而 `NewStores(tx)` 是让这一切成立的魔术——把整套 Repo 绑定到事务连接上。**
