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

### 3.1 internal/dao/redis/redis.go

> **路径确认**：Redis 模块已归类到 DAO 层 (`internal/dao/redis`)

```go
package redis

import (
	"context"
	"errors"
	"kama_chat_server/internal/config"
	"kama_chat_server/pkg/errorx"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client
var ctx = context.Background()

// Init 初始化 Redis 连接
func Init() {
	conf := config.GetConfig()
	host := conf.RedisConfig.Host
	port := conf.RedisConfig.Port
	password := conf.RedisConfig.Password
	db := conf.Db
	addr := host + ":" + strconv.Itoa(port)

	redisClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
}

// ==================== 基础操作 ====================

// SetKeyEx 设置带过期时间的键值
func SetKeyEx(key string, value string, timeout time.Duration) error {
	if err := redisClient.Set(ctx, key, value, timeout).Err(); err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis set key %s", key)
	}
	return nil
}

// GetKey 获取键值（键不存在时返回空字符串，不报错）
func GetKey(key string) (string, error) {
	value, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil  // 键不存在返回空字符串，不视为错误
		}
		return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis get key %s", key)
	}
	return value, nil
}

// GetKeyNilIsErr 获取键值（键不存在时返回 CodeNotFound 错误）
func GetKeyNilIsErr(key string) (string, error) {
	value, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", errorx.Wrapf(err, errorx.CodeNotFound, "redis key %s not found", key)
		}
		return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis get key %s", key)
	}
	return value, nil
}
```

**错误处理设计**：
- 使用 `errorx.Wrapf` 包装原始错误，保留上下文信息
- `CodeCacheError` 用于缓存故障
- `CodeNotFound` 用于键不存在的情况
- Service 层可通过 `errorx.GetCode(err) == errorx.CodeNotFound` 判断

### 3.2 模式匹配查询函数

项目还提供了按前缀/后缀查找的工具函数：

```go
// GetKeyWithPrefixNilIsErr 根据前缀查找唯一键
func GetKeyWithPrefixNilIsErr(prefix string) (string, error) {
	var cursor uint64
	var foundKeys []string
	for {
		keys, cursor, err := redisClient.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return "", errorx.Wrapf(err, errorx.CodeCacheError, "redis scan prefix %s", prefix)
		}
		foundKeys = append(foundKeys, keys...)
		if len(foundKeys) > 1 {
			return "", errorx.Newf(errorx.CodeCacheError, "found %d keys, expected 1", len(foundKeys))
		}
		if cursor == 0 {
			break
		}
	}
	if len(foundKeys) == 0 {
		return "", errorx.Wrapf(redis.Nil, errorx.CodeNotFound, "redis prefix %s not found", prefix)
	}
	return foundKeys[0], nil
}

// GetKeyWithSuffixNilIsErr 根据后缀查找唯一键
func GetKeyWithSuffixNilIsErr(suffix string) (string, error) {
	// 类似 GetKeyWithPrefixNilIsErr，使用 "*"+suffix 模式
	// ...
}
```

### 3.3 删除操作

```go
// DelKeyIfExists 删除存在的键（不存在也不报错）
func DelKeyIfExists(key string) error {
	exists, err := redisClient.Exists(ctx, key).Result()
	if err != nil {
		return errorx.Wrapf(err, errorx.CodeCacheError, "redis exists key %s", key)
	}
	if exists == 1 {
		if err := redisClient.Del(ctx, key).Err(); err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis delete key %s", key)
		}
	}
	return nil
}

// DelKeysWithPattern 删除匹配模式的键（使用 Scan + Unlink）
func DelKeysWithPattern(pattern string) error {
	var cursor uint64
	for {
		keys, cursor, err := redisClient.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return errorx.Wrapf(err, errorx.CodeCacheError, "redis scan pattern %s", pattern)
		}
		if len(keys) > 0 {
			// 使用 Unlink 进行非阻塞异步删除
			if err := redisClient.Unlink(ctx, keys...).Err(); err != nil {
				return errorx.Wrapf(err, errorx.CodeCacheError, "redis unlink keys")
			}
		}
		if cursor == 0 {
			break
		}
	}
	return nil
}

// DelKeysWithPatterns 批量删除多个模式匹配的 key
func DelKeysWithPatterns(patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}
	for _, pattern := range patterns {
		if err := DelKeysWithPattern(pattern); err != nil {
			return err
		}
	}
	return nil
}

// DelKeysWithPrefix 删除所有匹配前缀的键
func DelKeysWithPrefix(prefix string) error {
	return DelKeysWithPattern(prefix + "*")
}

// DelKeysWithSuffix 删除所有匹配后缀的键
func DelKeysWithSuffix(suffix string) error {
	return DelKeysWithPattern("*" + suffix)
}

// DeleteAllRedisKeys 删除所有键（危险操作，仅用于测试）
func DeleteAllRedisKeys() error {
	var cursor uint64 = 0
	for {
		keys, nextCursor, err := redisClient.Scan(ctx, cursor, "*", 0).Result()
		if err != nil {
			return errorx.Wrap(err, errorx.CodeCacheError, "redis scan all keys")
		}
		if len(keys) > 0 {
			if _, err := redisClient.Del(ctx, keys...).Result(); err != nil {
				return errorx.Wrap(err, errorx.CodeCacheError, "redis delete all keys")
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}
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

	// 3. 初始化数据库
	dao.Init()
	zap.L().Info("数据库初始化成功")

	// 4. 初始化 Redis
	myredis.Init()
	zap.L().Info("Redis 初始化成功")

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
	myredis "kama_chat_server/internal/dao/redis"
	"time"
)

// 存储验证码，5分钟过期
func SaveAuthCode(telephone, code string) error {
	key := fmt.Sprintf("auth_code:%s", telephone)
	return myredis.SetKeyEx(key, code, 5*time.Minute)
}

// 验证验证码
func VerifyAuthCode(telephone, inputCode string) (bool, error) {
	key := fmt.Sprintf("auth_code:%s", telephone)
	savedCode, err := myredis.GetKey(key)
	if err != nil {
		return false, err
	}
	if savedCode == "" {
		return false, errors.New("验证码已过期")
	}
	return savedCode == inputCode, nil
}
```

### 5.2 用户在线状态管理

```go
// 设置用户在线状态（1小时过期）
func SetUserOnline(uuid string) error {
	key := fmt.Sprintf("online:%s", uuid)
	return myredis.SetKeyEx(key, "1", 1*time.Hour)
}

// 检查用户是否在线
func IsUserOnline(uuid string) (bool, error) {
	key := fmt.Sprintf("online:%s", uuid)
	value, err := myredis.GetKey(key)
	if err != nil {
		return false, err
	}
	return value != "", nil
}

// 用户下线
func SetUserOffline(uuid string) error {
	key := fmt.Sprintf("online:%s", uuid)
	return myredis.DelKeyIfExists(key)
}
```

### 5.3 消息缓存

```go
// 缓存最近的聊天消息（可配置过期时间）
func CacheMessage(sendId, receiveId, messageContent string) error {
	key := fmt.Sprintf("message_cache:%s:%s", sendId, receiveId)
	// 缓存24小时
	return myredis.SetKeyEx(key, messageContent, 24*time.Hour)
}
```

---

## 6. Redis 键命名规范

| 类型 | 格式 | 示例 |
|-----|------|------|
| 验证码 | `auth_code:{telephone}` | `auth_code:13800138000` |
| 在线状态 | `online:{uuid}` | `online:U1234567890` |
| 消息缓存 | `message_list_{sendId}_{receiveId}` | `message_list_U123_U456` |
| 群消息缓存 | `group_messagelist_{groupId}` | `group_messagelist_G789` |
| 用户信息 | `user_info_{uuid}` | `user_info_U123456` |
| 群组信息 | `group_info_{groupId}` | `group_info_G789` |

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

	// 1. 尝试从 Redis 缓存获取
	rspString, err := myredis.GetKey(key)
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
	myredis.SetKeyEx(key, string(jsonData), time.Hour)

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
	myredis.DelKeyIfExists("user_info_" + updateReq.Uuid)

	return nil
}

// DisableUsers 禁用用户 (批量 + 异步缓存清理)
func (u *userInfoService) DisableUsers(uuidList []string) error {
	// 1. 批量更新用户状态
	if err := u.repos.User.UpdateUserStatusByUuids(uuidList, user_status_enum.DISABLE); err != nil {
		return errorx.ErrServerBusy
	}

	// 2. 异步清除 Redis 缓存 (不阻塞主流程)
	go func(uuids []string) {
		var patterns []string
		for _, uuid := range uuids {
			patterns = append(patterns,
				"user_info_"+uuid,
				"direct_session_list_"+uuid+"*",
				"group_session_list_"+uuid+"*",
			)
		}
		myredis.DelKeysWithPatterns(patterns)
	}(uuidList)

	return nil
}
```

### 7.3 注意事项

⚠️ **不要使用 `Keys()` 在生产环境**：
```go
// ❌ 错误 - Keys() 会阻塞 Redis
keys, _ := redisClient.Keys(ctx, "user_*").Result()

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
myredis.SetKeyEx(key, value, time.Hour + randomOffset)
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
