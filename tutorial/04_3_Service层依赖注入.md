# 04_3. Service层依赖注入

> 本教程介绍 KamaChat 项目中 Service 层的依赖注入（Dependency Injection）架构设计与实现。

---

## 📌 学习目标

- 理解依赖注入的核心概念和优势
- 掌握 Service 层的接口设计
- 了解 Service 聚合（Services）与构造入口（NewServices）
- 学会在 Handler 层通过构造函数注入调用 Service

---

## 1. 为什么需要依赖注入？

### 1.1 传统方式的问题

在重构之前，项目采用全局单例模式：

```go
// ❌ 旧模式 - 全局变量
package user

type userInfoService struct{}
var Service = new(userInfoService)

func (u *userInfoService) GetUserInfo(uuid string) (*User, error) {
    // 直接访问全局 DAO
    return dao.Repos.User.FindByUuid(uuid)
}
```

**问题**：
- **紧耦合**：Service 直接依赖具体的 `dao.Repos` 实现
- **难以测试**：无法 Mock 数据库层进行单元测试
- **隐式依赖**：依赖关系不清晰，难以追踪

### 1.2 依赖注入的优势

```go
// ✅ 新模式 - 显式注入 repos + cache
type userInfoService struct {
    repos *repository.Repositories
    cache myredis.AsyncCacheService
}

func NewUserService(repos *repository.Repositories, cache myredis.AsyncCacheService) *userInfoService {
    return &userInfoService{repos: repos, cache: cache}
}
```

**优势**：
- **松耦合**：依赖接口而非具体实现
- **易测试**：可注入 Mock 实现
- **显式依赖**：统一通过 `repos` 字段访问所有 Repository
- **事务支持**：可用 `repos.Transaction()` 启动事务

---

## 2. 架构设计

### 2.1 整体架构图

```
┌─────────────────────────────────────────────────────────────┐
│                      Handler 层                              │
│   Router 注入 handlers，Handler 持有 Service 接口            │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                 service.NewServices (DI 入口)                 │
│   ┌─────────────┐  ┌──────────────┐  ┌─────────────────┐    │
│   │ UserService │  │ GroupService │  │ ContactService   │    │
│   └──────┬──────┘  └──────┬───────┘  └────────┬────────┘    │
│          │                │                    │             │
│          └────────────────┼────────────────────┘             │
│                           │                                  │
│             依赖注入 repos + cacheService                      │
└───────────────────────────┼─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    Repository 层                             │
│         UserRepo / GroupRepo / SessionRepo / ...            │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 核心文件结构

```
internal/service/
├── interfaces.go      # ⭐ 接口 + Services 聚合 + NewServices 入口
├── user/
│   └── service.go     # UserService 具体实现
├── auth/
│   └── service.go     # AuthService 具体实现
├── group/
│   └── service.go     # GroupService 具体实现
├── contact/
│   └── service.go     # ContactService 具体实现
├── session/
│   └── service.go     # SessionService 具体实现
├── message/
│   └── service.go     # MessageService 具体实现
└── chat/              # ChatServer/WebSocket/MQ（独立于 Services，由 main 负责初始化）
    ├── server.go
    ├── ws_gateway.go
    ├── channel_broker.go
    ├── kafka_broker.go
    └── kafka_client.go
```

---

## 3. 接口定义（interfaces.go）

### 3.1 Service 接口示例

```go
// 位置: internal/service/interfaces.go
package service

import (
    "github.com/gin-gonic/gin"

    "kama_chat_server/internal/dao/mysql/repository"
    myredis "kama_chat_server/internal/dao/redis"
    "kama_chat_server/internal/dto/request"
    "kama_chat_server/internal/dto/respond"
)

// UserService 用户业务接口
type UserService interface {
    Login(req request.LoginRequest) (*respond.LoginRespond, error)
    SmsLogin(req request.SmsLoginRequest) (*respond.LoginRespond, error)
    SendSmsCode(telephone string) error
    Register(req request.RegisterRequest) (*respond.RegisterRespond, error)
    UpdateUserInfo(req request.UpdateUserInfoRequest) error
    GetUserInfoList(ownerId string) ([]respond.GetUserListRespond, error)
    AbleUsers(uuidList []string) error
    DisableUsers(uuidList []string) error
    DeleteUsers(uuidList []string) error
    GetUserInfo(uuid string) (*respond.GetUserInfoRespond, error)
    SetAdmin(uuidList []string, isAdmin int8) error
}

// SessionService 会话业务接口
type SessionService interface {
    CreateSession(req request.CreateSessionRequest) (string, error)
    CheckOpenSessionAllowed(sendId, receiveId string) (bool, error)
    OpenSession(req request.OpenSessionRequest) (string, error)
    GetUserSessionList(ownerId string) ([]respond.UserSessionListRespond, error)
    GetGroupSessionList(ownerId string) ([]respond.GroupSessionListRespond, error)
    DeleteSession(ownerId, sessionId string) error
}

// GroupService 群组业务接口
type GroupService interface {
    CreateGroup(req request.CreateGroupRequest) error
    LoadMyGroup(ownerId string) ([]respond.LoadMyGroupRespond, error)
    CheckGroupAddMode(groupId string) (int8, error)
    EnterGroupDirectly(groupId, userId string) error
    LeaveGroup(userId, groupId string) error
    DismissGroup(ownerId, groupId string) error
    GetGroupInfo(groupId string) (*respond.GetGroupInfoRespond, error)
    GetGroupInfoList(req request.GetGroupListRequest) (*respond.GetGroupListWrapper, error)
    DeleteGroups(uuidList []string) error
    SetGroupsStatus(uuidList []string, status int8) error
    UpdateGroupInfo(req request.UpdateGroupInfoRequest) error
    GetGroupMemberList(groupId string) ([]respond.GetGroupMemberListRespond, error)
    RemoveGroupMembers(req request.RemoveGroupMembersRequest) error
}

// ContactService 联系人业务接口
type ContactService interface {
    // 好友/群详情
    GetFriendInfo(friendId string) (respond.GetFriendInfoRespond, error)
    GetGroupDetail(groupId string) (respond.GetGroupDetailRespond, error)

    // 联系人列表
    GetUserList(userId string) ([]respond.MyUserListRespond, error)
    GetJoinedGroupsExcludedOwn(userId string) ([]respond.LoadMyJoinedGroupRespond, error)
    DeleteContact(userId, contactId string) error

    // 好友申请
    ApplyFriend(req request.ApplyFriendRequest) error
    GetFriendApplyList(userId string) ([]respond.NewContactListRespond, error)
    PassFriendApply(userId, applicantId string) error
    RefuseFriendApply(userId, applicantId string) error
    BlackFriendApply(userId, applicantId string) error

    // 入群申请
    ApplyGroup(req request.ApplyGroupRequest) error
    GetGroupApplyList(groupId string) ([]respond.AddGroupListRespond, error)
    PassGroupApply(groupId, applicantId string) error
    RefuseGroupApply(groupId, applicantId string) error
    BlackGroupApply(groupId, applicantId string) error

    // 联系人状态
    BlackContact(userId, contactId string) error
    CancelBlackContact(userId, contactId string) error
}

// MessageService 消息业务接口
type MessageService interface {
    GetMessageList(userOneId, userTwoId string) ([]respond.GetMessageListRespond, error)
    GetGroupMessageList(groupId string) ([]respond.GetGroupMessageListRespond, error)
    UploadAvatar(c *gin.Context) (string, error)
    UploadFile(c *gin.Context) ([]string, error)
}

// AuthService 认证业务接口（用于单点登录互踢等）
type AuthService interface {
    ValidateTokenID(userID, tokenID string) (bool, error)
}

// Services 聚合所有 Service 实例
type Services struct {
    User    UserService
    Session SessionService
    Group   GroupService
    Contact ContactService
    Message MessageService
    Auth    AuthService
}

// NewServices 创建并注入所有 Service 实例
func NewServices(repos *repository.Repositories, cacheService myredis.AsyncCacheService) *Services {
    // ...
}

// 注：当前项目没有 ChatRoomService（聊天室功能已并入 chat 子系统）。
```

### 3.2 接口设计原则

1. **单一职责**：每个接口只定义一个业务领域的方法
2. **依赖倒置**：Handler 层依赖接口，不依赖具体实现
3. **接口隔离**：按功能模块拆分，避免过大的接口

---

## 4. 具体实现（以 UserService 为例）

### 4.1 结构体定义

```go
// 位置: internal/service/user/service.go
package user

import (
    "kama_chat_server/internal/dao/mysql/repository"
    "kama_chat_server/internal/dto/request"
    "kama_chat_server/internal/dto/respond"
)

// userInfoService 用户服务实现
type userInfoService struct {
    repos *repository.Repositories
    cache myredis.AsyncCacheService
}

// NewUserService 构造函数 - 注入 repos + cache
func NewUserService(repos *repository.Repositories, cache myredis.AsyncCacheService) *userInfoService {
    return &userInfoService{repos: repos, cache: cache}
}
```

### 4.2 方法实现

```go
// GetUserInfo 获取用户信息
func (u *userInfoService) GetUserInfo(uuid string) (*respond.GetUserInfoRespond, error) {
    // 使用 u.repos.XXX 访问各 Repository
    user, err := u.repos.User.FindByUuid(uuid)
    if err != nil {
        return nil, errorx.ErrServerBusy
    }
    
    return &respond.GetUserInfoRespond{
        Uuid:     user.Uuid,
        Nickname: user.Nickname,
        Avatar:   user.Avatar,
    }, nil
}

// DeleteUsers 批量删除用户 (带事务)
func (u *userInfoService) DeleteUsers(uuidList []string) error {
    // 使用事务确保原子性
    return u.repos.Transaction(func(txRepos *repository.Repositories) error {
        // 1. 批量软删除用户
        if err := txRepos.User.SoftDeleteUserByUuids(uuidList); err != nil {
            return errorx.ErrServerBusy
        }
        
        // 2. 批量删除相关会话
        if err := txRepos.Session.SoftDeleteByUsers(uuidList); err != nil {
            return errorx.ErrServerBusy
        }
        
        // 3. 批量删除联系人关系
        if err := txRepos.Contact.SoftDeleteByUsers(uuidList); err != nil {
            return errorx.ErrServerBusy
        }
        
        return nil
    })
}
```

---

## 5. Services 聚合（NewServices）

> **更新说明**：当前项目不再使用 `provider.go` + `service.Svc` 的全局入口。
> Service 聚合与构造入口统一放在 `internal/service/interfaces.go` 的 `Services` / `NewServices` 中，并由 `main.go` 显式创建后注入到 Handler。

### 5.1 Services 聚合结构（当前实现）

```go
// 位置: internal/service/interfaces.go
package service

import (
    "kama_chat_server/internal/dao/mysql/repository"
    myredis "kama_chat_server/internal/dao/redis"
    "kama_chat_server/internal/service/auth"
    "kama_chat_server/internal/service/contact"
    "kama_chat_server/internal/service/group"
    "kama_chat_server/internal/service/message"
    "kama_chat_server/internal/service/session"
    "kama_chat_server/internal/service/user"
)

// Services 聚合所有 Service 实例
type Services struct {
    User    UserService
    Session SessionService
    Group   GroupService
    Contact ContactService
    Message MessageService
    Auth    AuthService
}

// NewServices 创建并注入所有 Service 实例
func NewServices(repos *repository.Repositories, cacheService myredis.AsyncCacheService) *Services {
    sessionSvc := session.NewSessionService(repos, cacheService)
    userSvc := user.NewUserService(repos, cacheService)
    groupSvc := group.NewGroupService(repos, cacheService)
    contactSvc := contact.NewContactService(repos, cacheService)
    messageSvc := message.NewMessageService(repos, cacheService)
    authSvc := auth.NewAuthService(cacheService)

    return &Services{
        User:    userSvc,
        Session: sessionSvc,
        Group:   groupSvc,
        Contact: contactSvc,
        Message: messageSvc,
        Auth:    authSvc,
    }
}
```

---

## 6. 应用初始化（main.go）

```go
// 位置: cmd/kama_chat_server/main.go
func main() {
    conf := config.GetConfig()
    logger.Init(&conf.LogConfig, "dev")

    repos := dao.Init()
    cacheService := myredis.Init()

    services := service.NewServices(repos, cacheService)
    chatServer := chat.NewChatServer(chat.ChatServerConfig{ /* ... */ })
    handlers := handler.NewHandlers(services, chatServer.GetBroker())

    engine := https_server.Init(handlers)
    engine.Run(":8000")
}
```

**关键点**：
- `dao.Init()` 返回 `*repository.Repositories`，由 main 显式持有并注入
- `service.NewServices(repos, cacheService)` 创建 Service 聚合
- `handler.NewHandlers(services, broker)` 将 Service 接口注入到各 Handler

---

## 7. Handler 层调用

### 7.1 正确的调用方式

```go
// 位置: internal/handler/user_handler.go
package handler

import (
    "kama_chat_server/internal/service"
    "github.com/gin-gonic/gin"
)

// UserHandler 获取用户信息
type UserHandler struct {
    userSvc service.UserService
}

func NewUserHandler(userSvc service.UserService) *UserHandler {
    return &UserHandler{userSvc: userSvc}
}

func (h *UserHandler) GetUserInfo(c *gin.Context) {
    var req request.GetUserInfoRequest
    if err := c.ShouldBindQuery(&req); err != nil {
        HandleParamError(c, err)
        return
    }
    data, err := h.userSvc.GetUserInfo(req.Uuid)
    if err != nil {
        HandleError(c, err)
        return
    }
    HandleSuccess(c, data)
}
```

### 7.2 对比新旧模式

```go
// ❌ 旧模式
import "kama_chat_server/internal/service/user"
data, err := user.Service.GetUserInfo(uuid)

// ✅ 新模式：Handler 持有接口，通过 h.userSvc 调用
data, err := h.userSvc.GetUserInfo(uuid)
```

---

## 8. 测试示例

依赖注入的最大优势是便于测试。可以创建 Mock 实现：

```go
// 测试文件: internal/service/user/service_test.go
package user_test

import (
    "testing"
    "kama_chat_server/internal/model"
    "kama_chat_server/internal/dao/mysql/repository"
)

// MockRepositories 模拟 Repository 聚合
type MockRepositories struct {
    repository.Repositories
    MockUser *MockUserRepository
}

// MockUserRepository 模拟用户 Repository
type MockUserRepository struct {
    FindByUuidFunc func(uuid string) (*model.UserInfo, error)
}

func (m *MockUserRepository) FindByUuid(uuid string) (*model.UserInfo, error) {
    return m.FindByUuidFunc(uuid)
}

// 其他方法的空实现...

func TestGetUserInfo(t *testing.T) {
    // 准备 Mock
    mockRepos := &repository.Repositories{
        User: &MockUserRepository{
            FindByUuidFunc: func(uuid string) (*model.UserInfo, error) {
                return &model.UserInfo{
                    Uuid:     "U123456",
                    Nickname: "TestUser",
                }, nil
            },
        },
    }
    
    // 注入 Mock 创建 Service（示例：传入一个实现 myredis.AsyncCacheService 的 stub）
    // type cacheStub struct{}
    // func (cacheStub) SubmitTask(action func()) { action() }
    // func (cacheStub) Set(ctx context.Context, key string, value string, ttl time.Duration) error { return nil }
    // ...（其余方法按需返回零值即可）
    // svc := user.NewUserService(mockRepos, cacheStub{})

    // 这里为了突出“通过构造函数注入依赖”的思路，省略 stub 的完整实现。
    svc := user.NewUserService(mockRepos, nil)
    
    // 执行测试
    result, err := svc.GetUserInfo("U123456")
    
    // 断言
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.Nickname != "TestUser" {
        t.Errorf("expected TestUser, got %s", result.Nickname)
    }
}
```

---

## 9. 最佳实践

### 9.1 设计原则

| 原则 | 说明 |
|------|------|
| **接口优先** | 先定义接口，再实现具体类型 |
| **构造函数注入** | 通过 `New*Service` 函数注入依赖 |
| **显式依赖** | 所有依赖都在构造函数参数中体现 |
| **集中管理** | 通过 `service.NewServices` 统一构造聚合实例 |

### 9.2 注意事项

1. **避免循环依赖**：Service A 不应依赖 Service B，如果有此需求，考虑抽取公共接口
2. **统一依赖入口**：每个 Service 注入 `repos + cacheService`，通过它访问 Repository/缓存
3. **避免全局入口**：当前项目直接把 `services` 注入到 `handlers`，不依赖 `service.Svc`

### 9.3 目录规范

```
internal/service/
├── interfaces.go      # 接口 + Services 聚合 + NewServices 入口
├── <module>/
│   └── service.go     # 模块实现（每个模块一个）
└── chat/              # ChatServer/WebSocket/MQ（由 main 单独初始化）
    ├── server.go
    ├── ws_gateway.go
    ├── channel_broker.go
    ├── kafka_broker.go
    └── kafka_client.go
```

---

## ✅ 本节完成

你已经学会了：

- [x] 理解依赖注入的优势
- [x] 定义 Service 接口
- [x] 实现构造函数注入
- [x] 使用 Services 聚合集中管理
- [x] 在 Handler 层正确调用 Service
- [x] 编写可测试的代码

---

## 📚 下一步

继续学习 [05_Redis缓存集成.md](05_Redis缓存集成.md)，了解 Redis 缓存的集成方式。
