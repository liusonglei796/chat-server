# Service 层依赖注入策略

## 背景

在 DDD（领域驱动设计）架构中，Service 层不应直接依赖基础设施层（如 `internal/dao/mysql`）的具体实现。所有 Repository 接口定义在 `internal/domain/repository` 包中，Service 层仅依赖这些接口。

本项目根据 Service 是否需要**数据库事务**，采用两种不同的依赖注入方式。

---

## 策略一：UnitOfWork（事务型 Service）

### 适用场景

当一个 Service 方法需要在**同一个数据库事务**中操作多张表时，使用 `UnitOfWork` 接口。

### 接口定义

```go
// internal/domain/repository/unit_of_work.go
type UnitOfWork interface {
    UserRepo() UserRepository
    GroupRepo() GroupRepository
    FriendshipRepo() FriendshipRepository
    SessionRepo() SessionRepository
    MessageRepo() MessageRepository
    ApplyRepo() ApplyRepository
    GroupMemberRepo() GroupMemberRepository
    Transaction(fn func(uow UnitOfWork) error) error
}
```

### 使用示例

```go
// internal/service/group/service.go
type GroupService struct {
    uow   repository.UnitOfWork   // 注入 UnitOfWork
    cache repository.AsyncCacheService
}

func (g *GroupService) CreateGroup(ctx context.Context, ...) error {
    // 非事务读取
    group, _ := g.uow.GroupRepo().FindByUuid(ctx, groupId)

    // 事务写入：创建群组 + 创建群成员 + 创建会话
    return g.uow.Transaction(func(tx repository.UnitOfWork) error {
        if err := tx.GroupRepo().CreateGroup(ctx, &group); err != nil {
            return err
        }
        if err := tx.GroupMemberRepo().CreateGroupMember(ctx, &member); err != nil {
            return err
        }
        return tx.SessionRepo().CreateSession(ctx, &session)
    })
}
```

### 关键点

- `Transaction()` 回调参数 `tx` 是绑定了事务连接的**新 UnitOfWork 实例**
- 回调内通过 `tx.XxxRepo()` 获取的 Repository 共享同一个数据库事务
- 回调返回 `nil` 则提交，返回 `error` 则回滚

### 适用的 Service

| Service | 事务操作示例 |
|---|---|
| `UserService` | 更新用户信息 + 同步会话冗余字段 |
| `GroupService` | 创建群组 + 添加群成员 + 创建会话 |
| `FriendshipService` | 拉黑好友 + 更新双方状态 + 删除会话 |
| `ApplyService` | 通过好友申请 + 双向创建好友关系 |
| `UserAdminService` | 禁用用户 + 批量删除会话 |
| `GroupAdminService` | 解散群组 + 删除成员 + 删除会话 |

---

## 策略二：单独接口注入（非事务型 Service）

### 适用场景

当一个 Service **不需要事务**，只做查询或单表写入时，直接注入它所依赖的 Repository 接口。

### 使用示例

```go
// internal/service/message/service.go
type MessageService struct {
    messageRepo    repository.MessageRepository
    friendshipRepo repository.FriendshipRepository
    sessionRepo    repository.SessionRepository
    cache          repository.AsyncCacheService
}

func (m *MessageService) GetPrivateMessages(ctx context.Context, ...) {
    isFriend, _ := m.friendshipRepo.IsFriend(ctx, userId, friendId)
    messages, _ := m.messageRepo.FindByUserIdsPaged(ctx, ...)
}
```

### 优势

1. **接口隔离原则（ISP）**：每个 Service 只看到它需要的接口，依赖关系一目了然
2. **可测试性**：单元测试时只需 mock 少量接口，而非整个 UnitOfWork
3. **编译时安全**：缺少依赖会在编译期报错，不会遗漏

### 适用的 Service

| Service | 注入的接口 |
|---|---|
| `SessionService` | Session, User, Group, GroupMember, Friendship, Message |
| `MessageService` | Message, Friendship, Session |
| `AiService` | Message, GroupMember, Session, Friendship |

---

## 如何选择？

```
需要在一个方法中跨多表写入且要求原子性？
├── 是 → 使用 UnitOfWork
└── 否 → 注入单独的 Repository 接口
```

## 依赖关系图

```
┌──────────────┐
│   main.go    │  ← 组装层：创建具体实现，传入接口
└──────┬───────┘
       │ 传入 *mysql.Repositories（实现了 UnitOfWork）
       ▼
┌──────────────┐     ┌─────────────────────────┐
│ services.go  │────▶│ domain/repository       │
│  (wiring)    │     │  ├── UnitOfWork          │
└──────────────┘     │  ├── UserRepository      │
                     │  ├── GroupRepository      │
  Service 层         │  ├── ...                  │
  ┌────────────┐     │  ├── CacheService        │
  │ UserSvc    │────▶│  └── AsyncCacheService   │
  │ (UnitOfWork)│    └─────────────────────────┘
  └────────────┘               ▲
  ┌────────────┐               │ 实现
  │ MessageSvc │──── 单独接口 ──┤
  │ (ISP)      │               │
  └────────────┘     ┌─────────┴───────────┐
                     │ dao/mysql            │
                     │  └── Repositories    │
                     │     (implements UOW) │
                     └─────────────────────┘
```
