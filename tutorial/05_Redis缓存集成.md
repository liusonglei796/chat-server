# 05. Redis 缓存集成

> 本教程将集成 Redis 缓存，用于存储在线状态、消息缓存和验证码等临时数据。

---

## 📌 学习目标

- 理解 Redis 在 IM 系统中的作用
- 实现 Redis 连接和常用操作
- 掌握缓存最佳实践

---

## 1. Redis 在 IM 系统中的应用

| 应用场景 | 说明 |
|---------|------|
| 在线状态 | 存储用户在线/离线状态 |
| 消息缓存 | 缓存最近消息，减少数据库查询 |
| 验证码 | 存储短信验证码（带过期时间） |
| 会话 Token | 用户登录 Token 管理 |
| 限流 | 接口请求频率限制 |

---

## 2. 安装依赖

```bash
go get github.com/redis/go-redis/v9
```

> **注意**：项目使用 `github.com/redis/go-redis/v9`（v9 版本）

---

## 3. 实现 Redis 服务

### 3.1 `internal/dao/redis`：接口 + 实现（支持 DI）

> **当前仓库的真实形态**：
> - 没有全局 `redisClient`，也不暴露包级 `SetKeyEx/GetKey` 这类函数
> - `Init()` 返回 `AsyncCacheService` 接口，供 Service/ChatServer 注入使用
> - 所有操作都显式传入 `context.Context`
> - 内置 worker pool：可用 `SubmitTask` 异步执行缓存更新/清理

#### 3.1.1 接口定义：`internal/dao/redis/interface.go`

```go
type CacheService interface {
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)         // key 不存在："", nil
	GetOrError(ctx context.Context, key string) (string, error)  // key 不存在：CodeNotFound
	GetByPrefix(ctx context.Context, prefix string) (string, error)

	Delete(ctx context.Context, key string) error
	DeleteByPattern(ctx context.Context, pattern string) error
	DeleteByPatterns(ctx context.Context, patterns []string) error

	AddToSet(ctx context.Context, key string, members ...interface{}) error
	GetSetMembers(ctx context.Context, key string) ([]string, error)
	RemoveFromSet(ctx context.Context, key string, members ...interface{}) error
}

type AsyncCacheService interface {
	CacheService
	SubmitTask(action func())
}
```

#### 3.1.2 初始化：`internal/dao/redis/init_redis.go`

```go
// Init 初始化 Redis 连接并返回缓存服务（用于依赖注入）
func Init() AsyncCacheService {
	conf := config.GetConfig()
	addr := conf.RedisConfig.Host + ":" + strconv.Itoa(conf.RedisConfig.Port)

	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     conf.RedisConfig.Password,
		DB:           conf.RedisConfig.Db,
		PoolSize:     50,
		MinIdleConns: 15,
	})

	return NewRedisCache(client, 15, 3000)
}
```

**错误处理设计**：
- 使用 `errorx.Wrapf` 包装原始错误，保留上下文信息
- `CodeCacheError` 用于缓存故障
- `CodeNotFound` 用于键不存在的情况
- Service 层可通过 `errorx.GetCode(err) == errorx.CodeNotFound` 判断

### 3.2 模式匹配查询（Scan）

当前实现提供了“按前缀唯一键”读取：`GetByPrefix(ctx, prefix)`，其内部使用 `SCAN prefix*`。

```go
value, err := cache.GetByPrefix(ctx, "auth_code:")
if err != nil {
	// 可能是 CodeNotFound 或 CodeCacheError
}
```

### 3.3 删除操作（Scan + Unlink）

```go
// 删除单个 key（不存在也不报错）
_ = cache.Delete(ctx, "user_info_"+uuid)

// 按 pattern 批量删除（内部是 Scan + Unlink）
_ = cache.DeleteByPattern(ctx, "direct_session_list_"+uuid+"*")

// 批量 patterns
_ = cache.DeleteByPatterns(ctx, []string{
	"user_info_" + uuid,
	"direct_session_list_" + uuid + "*",
	"group_session_list_" + uuid + "*",
})
```

**批量删除的优势**：
- 使用 `Unlink` 替代 `Del`，实现异步删除，不阻塞 Redis 主线程
- 使用 `Scan` 替代 `Keys`，避免阻塞 Redis
- 每次扫描 500 条，减少循环次数

---

## 4. 更新 main.go

更新 `cmd/kama_chat_server/main.go`：

```go
package main

import (
	"fmt"
	"log"

	"go.uber.org/zap"
	"kama_chat_server/internal/config"
	dao "kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/infrastructure/logger"
)

func main() {
	fmt.Println("KamaChat Server Starting...")

	// 1. 加载配置
	cfg := config.GetConfig()

	// 2. 初始化日志
	if err := logger.Init(&cfg.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}
	defer logger.Sync()

	// 3. 初始化数据库（返回 Repositories，用于依赖注入）
	repos := dao.Init()
	zap.L().Info("数据库初始化成功")

	// 4. 初始化 Redis（返回 AsyncCacheService，用于依赖注入）
	cacheService := myredis.Init()
	zap.L().Info("Redis 初始化成功")

	// 5. 初始化 Service 层（示例：把 repos/cacheService 注入进去）
	_ = service.NewServices(repos, cacheService)

	// TODO: 后续步骤
	// 5. 启动服务

	zap.L().Info("所有服务初始化完成")
}
```

**注意导入别名**：
- `dao "kama_chat_server/internal/dao/mysql"` - MySQL DAO
- `myredis "kama_chat_server/internal/dao/redis"` - Redis DAO（避免与 `redis` 包名冲突）

---

## 5. 使用示例

### 5.1 存储验证码

```go
import (
	"context"
	"fmt"
	"time"

	myredis "kama_chat_server/internal/dao/redis"
)

// 存储验证码，5分钟过期（示例：把 cacheService 作为依赖传入）
func SaveAuthCode(cacheService myredis.AsyncCacheService, telephone, code string) error {
	key := fmt.Sprintf("auth_code:%s", telephone)
	return cacheService.Set(context.Background(), key, code, 5*time.Minute)
}

// 验证验证码
func VerifyAuthCode(cacheService myredis.AsyncCacheService, telephone, inputCode string) (bool, error) {
	key := fmt.Sprintf("auth_code:%s", telephone)
	savedCode, err := cacheService.Get(context.Background(), key)
	if err != nil {
		return false, err
	}
	if savedCode == "" {
		return false, fmt.Errorf("验证码已过期")
	}
	return savedCode == inputCode, nil
}
```

### 5.2 用户在线状态管理

```go
// 设置用户在线状态（1小时过期）
func SetUserOnline(cacheService myredis.AsyncCacheService, uuid string) error {
	key := fmt.Sprintf("online:%s", uuid)
	return cacheService.Set(context.Background(), key, "1", 1*time.Hour)
}

// 检查用户是否在线
func IsUserOnline(cacheService myredis.AsyncCacheService, uuid string) (bool, error) {
	key := fmt.Sprintf("online:%s", uuid)
	value, err := cacheService.Get(context.Background(), key)
	if err != nil {
		return false, err
	}
	return value != "", nil
}

// 用户下线
func SetUserOffline(cacheService myredis.AsyncCacheService, uuid string) error {
	key := fmt.Sprintf("online:%s", uuid)
	return cacheService.Delete(context.Background(), key)
}
```

### 5.3 消息缓存

```go
// 缓存最近的聊天消息（可配置过期时间）
func CacheMessage(cacheService myredis.AsyncCacheService, sendId, receiveId, messageContent string) error {
	key := fmt.Sprintf("message_list_%s_%s", sendId, receiveId)
	// 示例：缓存 24 小时
	return cacheService.Set(context.Background(), key, messageContent, 24*time.Hour)
}
```

---

## 6. Redis 键命名规范

| 类型 | 格式 | 示例 |
|-----|------|------|
| 验证码 | `auth_code:{telephone}` | `auth_code:13800138000` |
| 在线状态 | `online:{uuid}` | `online:U1234567890` |
| 消息列表缓存 | `message_list_{sendId}_{receiveId}` | `message_list_U123_U456` |
| 群消息缓存 | `group_messagelist_{groupId}` | `group_messagelist_G789` |
| 用户信息 | `user_info_{uuid}` | `user_info_U123456` |
| 群组信息 | `group_info_{groupId}` | `group_info_G789` |
| 好友关系集合 | `contact_relation:user:{uuid}` | `contact_relation:user:U123` |
| 入群关系集合 | `contact_relation:group:{uuid}` | `contact_relation:group:U123` |

**命名原则**：
- 使用冒号 `:` 或下划线 `_` 分隔层级
- 保持一致性
- 见名知意

---

## 7. 缓存一致性模式

### 7.1 Cache-Aside (旁路缓存) 模式

**读取流程**：
1. 尝试从 Redis 读取数据
2. 缓存命中 → 直接返回
3. 缓存未命中 → 查询数据库 → 回写 Redis → 返回数据

**写入流程**：
1.  更新数据库
2.  **删除**缓存（不是更新缓存）

**为什么删除而不是更新？**
- 更新可能失败导致不一致
- 更新可能产生竞态条件
- 删除让下次读取自动刷新，逻辑简单

### 7.2 实际应用：用户信息缓存

#### 写入缓存 (GetUserInfo)

```go
func (u *userInfoService) GetUserInfo(uuid string) (*respond.GetUserInfoRespond, error) {
	key := "user_info_" + uuid

	// 1. 尝试从 Redis 缓存获取（通过注入的 cacheService）
	rspString, err := u.cache.Get(context.Background(), key)
	if err == nil && rspString != "" {
		var rsp respond.GetUserInfoRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return &rsp, nil  // 缓存命中，直接返回
		}
	}

	// 2. 缓存未命中，查询数据库
	user, err := u.repos.User.FindByUuid(uuid)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 3. 构造响应
	rsp := &respond.GetUserInfoRespond{
		Uuid:     user.Uuid,
		Nickname: user.Nickname,
		// ...其他字段
	}

	// 4. 回写缓存 (设置过期时间 1 小时)
	jsonData, _ := json.Marshal(rsp)
	_ = u.cache.Set(context.Background(), key, string(jsonData), time.Hour)

	return rsp, nil
}
```

#### 删除缓存 (UpdateUserInfo / DisableUsers)

```go
// UpdateUserInfo 修改用户信息
func (u *userInfoService) UpdateUserInfo(updateReq request.UpdateUserInfoRequest) error {
	// 1. 更新数据库
	user, _ := u.repos.User.FindByUuid(updateReq.Uuid)
	user.Nickname = updateReq.Nickname
	u.repos.User.UpdateUserInfo(user)

	// 2. 删除缓存（保证下次读取时拿到最新数据）
	_ = u.cache.Delete(context.Background(), "user_info_"+updateReq.Uuid)

	return nil
}

// DisableUsers 禁用用户 (批量 + 异步缓存清理)
func (u *userInfoService) DisableUsers(uuidList []string) error {
	// 1. 批量更新用户状态
	if err := u.repos.User.UpdateUserStatusByUuids(uuidList, user_status_enum.DISABLE); err != nil {
		return errorx.ErrServerBusy
	}

	// 2. 异步清除 Redis 缓存 (不阻塞主流程)
	u.cache.SubmitTask(func() {
		var patterns []string
		for _, uuid := range uuids {
			patterns = append(patterns,
				"user_info_"+uuid,
				"direct_session_list_"+uuid+"*",
				"group_session_list_"+uuid+"*",
			)
		}
		_ = u.cache.DeleteByPatterns(context.Background(), patterns)
	})

	return nil
}
```

### 7.3 注意事项

⚠️ **不要使用 `KEYS` 在生产环境**：
```go
// ❌ 错误 - KEYS 会阻塞 Redis
keys, _ := client.Keys(ctx, "user_*").Result()

// ✅ 正确 - 使用 Scan 逐步遍历
var cursor uint64
for {
    keys, nextCursor, _ := redisClient.Scan(ctx, cursor, "user_*", 100).Result()
    // 处理 keys...
    cursor = nextCursor
    if cursor == 0 {
        break
    }
}
```

⚠️ **缓存雪崩防护**：
```go
// 使用随机过期时间，避免大量缓存同时失效
randomOffset := time.Duration(rand.Intn(300)) * time.Second
_ = cache.Set(ctx, key, value, time.Hour+randomOffset)
```

---

## ✅ 本节完成

你已经完成了：
- [x] Redis 连接初始化
- [x] 基础操作封装
- [x] 模式匹配查询与删除
- [x] Redis DAO 层集成

---

## 📚 下一步

继续学习 [06_用户模型设计.md](06_用户模型设计.md)，开始 **阶段二：数据模型层**。
