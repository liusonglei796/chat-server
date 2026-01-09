# 04.2 DAO层与 Repository 模式

> 本教程将补充讲解如何实现 Data Access Object (DAO) 层，通过 Repository 模式封装数据库操作。

---

## 📌 学习目标

- 理解 Repository 接口设计
- 实现具体的 Repository 类
- 掌握 GORM 的 CRUD 操作封装
- 依赖注入与调用方式

---

## 1. 为什么需要 Repository 模式？

直接在 Service 层使用 `gorm.DB` 会导致业务逻辑与数据库实现强耦合。通过 Repository 模式，我们可以：

1.  **解耦**：Service 层只依赖接口，不关心底层是 MySQL 还是 PostgreSQL。
2.  **复用**：常用的查询逻辑封装在 Repository 中。
3.  **可测试性**：方便 Mock 数据库接口进行单元测试。

---

## 2. 定义 Repository 接口

所有的 Repository 接口都定义在 `internal/dao/mysql/repository/interfaces.go` 中。

> **路径变更**：从 `internal/dao/repository/` 改为 `internal/dao/mysql/repository/`

### 2.1 接口聚合结构体

我们定义一个全局的 `Repositories` 结构体，包含所有的 Repository 接口：

```go
package repository

import "gorm.io/gorm"

// Repositories 聚合所有 Repository
// Service 层通过注入 *Repositories 访问数据层。
type Repositories struct {
	db          *gorm.DB
	User        UserRepository
	Group       GroupRepository
	Contact     ContactRepository
	Session     SessionRepository
	Message     MessageRepository
	Apply       ApplyRepository
	GroupMember GroupMemberRepository
}

func NewRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		db:          db,
		User:        NewUserRepository(db),
		Group:       NewGroupRepository(db),
		Contact:     NewContactRepository(db),
		Session:     NewSessionRepository(db),
		Message:     NewMessageRepository(db),
		Apply:       NewApplyRepository(db),
		GroupMember: NewGroupMemberRepository(db),
	}
}

// Transaction 在事务中执行函数
func (r *Repositories) Transaction(fn func(txRepos *Repositories) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(NewRepositories(tx))
	})
}
```

**设计要点**：
- 所有 Repository 接口聚合在一个结构体中
- 通过 `NewRepositories` 工厂函数统一初始化
- 包含 `db` 字段以支持事务操作
- 通过 `Transaction()` 方法实现事务支持
- Service 层通过构造函数注入，如 `NewUserService(repos)`

### 2.2 用户接口示例

```go
// UserRepository 用户数据访问接口
type UserRepository interface {
	FindByUuid(uuid string) (*model.UserInfo, error)
	FindByTelephone(telephone string) (*model.UserInfo, error)
	FindAllExcept(excludeUuid string) ([]model.UserInfo, error)
	FindByUuids(uuids []string) ([]model.UserInfo, error)
	CreateUser(user *model.UserInfo) error
	UpdateUserInfo(user *model.UserInfo) error
	UpdateUserStatusByUuids(uuids []string, status int8) error   // 批量更新状态
	UpdateUserIsAdminByUuids(uuids []string, isAdmin int8) error // 批量设置管理员
	SoftDeleteUserByUuids(uuids []string) error                  // 批量软删除
}
```

**接口方法分类**：
- **查询方法**：`FindByXxx` - 根据不同条件查找
- **创建方法**：`CreateUser` - 创建新记录
- **更新方法**：`UpdateUserInfo`、`UpdateUserStatusByUuids` - 单个/批量更新
- **删除方法**：`SoftDeleteUserByUuids` - 批量软删除（保留数据）

> **注意**：为了解决 N+1 问题，项目使用批量操作替代循环单个操作。

### 2.3 其他 Repository 接口

完整的接口定义包括：

**GroupRepository** - 群组数据访问
```go
type GroupRepository interface {
	FindByUuid(uuid string) (*model.GroupInfo, error)
	FindByOwnerId(ownerId string) ([]model.GroupInfo, error)
	FindAll() ([]model.GroupInfo, error)
	FindByUuids(uuids []string) ([]model.GroupInfo, error)
	GetGroupList(page, pageSize int) ([]model.GroupInfo, int64, error)  // 分页查询
	CreateGroup(group *model.GroupInfo) error
	Update(group *model.GroupInfo) error
	UpdateStatusByUuids(uuids []string, status int8) error  // 批量更新状态
	IncrementMemberCount(uuid string) error                 // 增加成员数
	DecrementMemberCountBy(uuid string, count int) error    // 减少指定数量成员
	SoftDeleteByUuids(uuids []string) error                 // 批量软删除
}
```

**ContactRepository** - 联系人关系
```go
type ContactRepository interface {
	FindByUserIdAndContactId(userId, contactId string) (*model.Contact, error)
	// FindByUserIdWithType 根据用户ID和联系人类型查找
	FindByUserIdAndType(userId string, contactType int8) ([]model.Contact, error)
	// FindUsersByContactId 根据联系人ID反向查找
	FindUsersByContactId(contactId string) ([]model.Contact, error)
	// Create 创建联系人关系
	CreateContact(contact *model.Contact) error
	// UpdateStatus 更新联系人状态（正常/拉黑等）
	UpdateStatus(userId, contactId string, status int8) error
	SoftDelete(userId, contactId string) error
	SoftDeleteByUsers(userUuids []string) error
}
```

**SessionRepository** - 会话管理
```go
type SessionRepository interface {
	FindBySendIdAndReceiveId(sendId, receiveId string) (*model.Session, error)
	FindBySendId(sendId string) ([]model.Session, error)
	CreateSession(session *model.Session) error
	SoftDeleteByUuids(uuids []string) error
	SoftDeleteByUsers(userUuids []string) error
	UpdateByReceiveId(receiveId string, updates map[string]interface{}) error
}
```

**其他接口**：
- `MessageRepository` - 消息记录（`FindByUserIds`、`FindByGroupId`）
- `ApplyRepository` - 申请（包含 `SoftDeleteByUsers` 批量方法，覆盖好友申请/入群申请）
- `GroupMemberRepository` - 群成员管理（包含 `DeleteByUserUuids`、`DeleteByGroupUuids`、`GetMemberIdsByGroupUuids`）

---

## 3. 错误处理辅助函数

Repository 层使用 `wrapDBError` 辅助函数包装错误，为 Service 层提供统一的错误码：

```go
// helper.go 中定义辅助函数
func wrapDBError(err error, msg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorx.Wrap(err, errorx.CodeNotFound, msg)
	}
	return errorx.Wrap(err, errorx.CodeDBError, msg)
}

func wrapDBErrorf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorx.Wrapf(err, errorx.CodeNotFound, format, args...)
	}
	return errorx.Wrapf(err, errorx.CodeDBError, format, args...)
}
```

**Service 层错误处理**：

```go
// Service 层使用 errorx.GetCode 判断 NotFound，不直接依赖 gorm
user, err := u.repos.User.FindByUuid(uuid)
if err != nil {
    if errorx.GetCode(err) == errorx.CodeNotFound {
        return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
    }
    zap.L().Error(err.Error())
    return nil, errorx.ErrServerBusy
}
```

> **重要**：Service 层不应导入 `gorm.io/gorm`，通过 `errorx.GetCode()` 判断错误类型

---

## 4. 实现 Repository

以 `internal/dao/mysql/repository/user_repository.go` 为例。

### 4.1 结构体定义

私有结构体实现接口，通过构造函数返回接口类型：

```go
package repository

import (
	"kama_chat_server/internal/model"
	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
}

// NewUserRepository 构造函数
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}
```

### 4.2 查询实现 (Retrieve)

```go
// FindByUuid 按 UUID 查找用户
func (r *userRepository) FindByUuid(uuid string) (*model.UserInfo, error) {
	var user model.UserInfo
	if err := r.db.First(&user, "uuid = ?", uuid).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询用户 uuid=%s", uuid)
	}
	return &user, nil
}

// FindByTelephone 按手机号查找用户
func (r *userRepository) FindByTelephone(telephone string) (*model.UserInfo, error) {
	var user model.UserInfo
	if err := r.db.Where("telephone = ?", telephone).First(&user).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询用户 telephone=%s", telephone)
	}
	return &user, nil
}

// FindAllExcept 查找除某人外的所有用户
func (r *userRepository) FindAllExcept(excludeUuid string) ([]model.UserInfo, error) {
	var users []model.UserInfo
	if err := r.db.Unscoped().Where("uuid != ?", excludeUuid).Find(&users).Error; err != nil {
		return nil, wrapDBError(err, "查询用户列表")
	}
	return users, nil
}
```

**GORM 查询技巧**：
- `First()` - 查询单条记录，未找到返回 `gorm.ErrRecordNotFound`
- `Find()` - 查询多条记录，未找到返回空切片（不报错）
- `Unscoped()` - 包含软删除的记录
- `Where("uuid IN ?", uuids)` - IN 查询，自动处理切片参数

### 4.3 创建实现 (Create)

```go
// CreateUser 创建用户
func (r *userRepository) CreateUser(user *model.UserInfo) error {
	return r.db.Create(user).Error
}
```

### 4.4 更新实现 (Update)

```go
// UpdateUserInfo 更新整个对象
func (r *userRepository) UpdateUserInfo(user *model.UserInfo) error {
	return r.db.Save(user).Error
}

// UpdateUserStatusByUuids 批量更新状态
func (r *userRepository) UpdateUserStatusByUuids(uuids []string, status int8) error {
	if len(uuids) == 0 {
		return nil
	}
	return r.db.Model(&model.UserInfo{}).
		Where("uuid IN ?", uuids).
		Update("status", status).Error
}
```

### 4.5 删除实现 (Delete)

```go
// SoftDeleteUserByUuids 批量软删除
func (r *userRepository) SoftDeleteUserByUuids(uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	// GORM 默认启用软删除，只要模型包含 DeletedAt 字段
	return r.db.Where("uuid IN ?", uuids).Delete(&model.UserInfo{}).Error
}
```

**批量操作的优势**：
- 使用 `WHERE uuid IN (...)` 一次执行，而不是循环单个删除
- 解决 N+1 问题，大幅提升性能
- 空切片时直接返回 nil，避免无效查询

---

## 5. 全局初始化与调用

### 5.1 在 main.go 中初始化并向下游注入

当前项目采用“构造函数注入”为主：DAO 层初始化返回 `*repository.Repositories`，由 `main.go` 拿到返回值后传给 Service/ChatServer/Handler。

```go
repos := dao.Init()
cacheService := myredis.Init()

services := service.NewServices(repos, cacheService)
handlers := handler.NewHandlers(services, chatServer.GetBroker())
```

### 5.2 在 Service 层调用

业务代码通过注入的 `repos` 访问 Repository：

```go
func (s *userInfoService) GetUserInfo(uuid string) (*respond.GetUserInfoRespond, error) {
	// 使用 s.repos.User 访问用户 Repository
	user, err := s.repos.User.FindByUuid(uuid)
	if err != nil {
		return nil, err
	}
	return &respond.GetUserInfoRespond{
		Uuid:     user.Uuid,
		Nickname: user.Nickname,
	}, nil
}
```

---

## 6. 常用技巧

### 6.1 事务处理

使用 `repos.Transaction` 方法实现事务：

```go
func (s *userInfoService) DeleteUsers(uuidList []string) error {
	return s.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 1. 批量软删除用户
		if err := txRepos.User.SoftDeleteUserByUuids(uuidList); err != nil {
			return err // 自动回滚
		}

		// 2. 批量删除相关会话
		if err := txRepos.Session.SoftDeleteByUsers(uuidList); err != nil {
			return err // 自动回滚
		}

		// 3. 批量删除联系人关系
		if err := txRepos.Contact.SoftDeleteByUsers(uuidList); err != nil {
			return err // 自动回滚
		}

		// 如果没有错误，自动提交
		return nil
	})
}
```

### 6.2 复杂查询

对于复杂查询（Join等），建议在 Repository 中封装好方法：

```go
// GroupMemberWithUserInfo 群成员详细信息（含用户资料）
type GroupMemberWithUserInfo struct {
	UserId   string `json:"userId"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// FindMembersWithUserInfo 查询群成员详细信息
func (r *groupMemberRepository) FindMembersWithUserInfo(groupUuid string) ([]GroupMemberWithUserInfo, error) {
	var members []GroupMemberWithUserInfo
	err := r.db.Table("group_member").
		Select("group_member.user_uuid as user_id, user_info.nickname, user_info.avatar").
		Joins("LEFT JOIN user_info ON group_member.user_uuid = user_info.uuid").
		Where("group_member.group_uuid = ?", groupUuid).
		Scan(&members).Error
	return members, err
}
```

---

## ✅ 本节完成

你已经掌握了：
- [x] 定义 Repository 接口
- [x] 使用 GORM 实现 CRUD 方法
- [x] 封装复杂查询逻辑
- [x] 使用 Transaction 实现事务
- [x] 错误包装与处理

---

## 📚 下一步

继续学习 [04_3_Service层依赖注入.md](04_3_Service层依赖注入.md)，了解 Service 层的依赖注入架构。
