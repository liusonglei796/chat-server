# KamaChat Server 架构优化方案

## 总览

本文档详细描述 KamaChat Server 的全面架构优化方案，分为 7 个阶段实施。

**预计总工作量**: 15-20 小时  
**最后更新**: 2026-01-31

---

## 目录

1. [Phase 1: WebSocket 连接管理优化](#phase-1-websocket-连接管理优化)
2. [Phase 2: 数据库事务管理](#phase-2-数据库事务管理)
3. [Phase 3: 配置管理强化](#phase-3-配置管理强化)
4. [Phase 4: 代码重构](#phase-4-代码重构)
5. [Phase 5: 缓存策略优化](#phase-5-缓存策略优化)
6. [Phase 6: 限流保护](#phase-6-限流保护)
7. [Phase 7: 可维护性改进](#phase-7-可维护性改进)

---

## 优化阶段总览

| 阶段 | 名称 | 优先级 | 预计时间 | 核心目标 |
|------|------|--------|----------|----------|
| 1 | WebSocket 连接管理 | 高 | 3-4h | 心跳、连接数限制、消息确认 |
| 2 | 数据库事务管理 | 高 | 2-3h | 关键业务原子性保证 |
| 3 | 配置管理强化 | 高 | 2h | 配置验证、阻止无效启动 |
| 4 | 代码重构 | 中 | 3-4h | 减少重复、提取公共逻辑 |
| 5 | 缓存策略优化 | 中 | 2-3h | 一致性、穿透、雪崩保护 |
| 6 | 限流保护 | 中 | 2-3h | API 和 WebSocket 限流 |
| 7 | 可维护性改进 | 低 | 2-3h | 测试、日志、文档 |

---

## Phase 1: WebSocket 连接管理优化

### 当前问题

#### 1. 无心跳机制
- **位置**: `internal/service/chat/ws_gateway.go:66-84`
- **问题**: `Read()` 方法循环读取消息，如果客户端静默断开（如断电、断网），服务端无法感知
- **风险**: 长期运行后积累大量僵尸连接，内存泄漏

#### 2. 无连接数限制
- **位置**: `internal/service/chat/channel_broker.go:114`
- **问题**: `s.Clients.Store(client.Uuid, client)` 直接注册，没有上限检查
- **风险**: 可能被恶意攻击导致资源耗尽

#### 3. 无消息确认机制
- **位置**: `internal/service/chat/channel_broker.go:388-393`
- **问题**: 消息推送到 `SendBack` 通道即认为成功，没有确认机制
- **风险**: 消息丢失无法感知

### 优化方案

#### 1. 心跳机制设计

**实现文件**: `internal/service/chat/ws_connection.go` (新建)

```go
type ConnectionManager struct {
    maxConnections        int32           // 最大连接数
    currentConnections    int32           // 当前连接数（原子操作）
    heartbeatInterval     time.Duration   // 心跳间隔（30秒）
    heartbeatTimeoutCount int             // 超时次数（3次）
}
```

**心跳流程**:
1. 服务端每 30 秒发送 Ping 帧
2. 客户端收到 Ping 后返回 Pong
3. 连续 3 次未收到 Pong，断开连接
4. 使用 `sync/atomic` 保证连接数统计线程安全

**代码修改位置**:
- `ws_gateway.go`: `NewClientInit()` 启动心跳检测 goroutine
- `UserConn` 结构体添加 `heartbeatStop` 通道

#### 2. 连接数限制设计

**配置项** (添加到 `configs/config.toml`):
```toml
[websocketConfig]
maxConnections = 10000        # 最大连接数
heartbeatInterval = 30        # 心跳间隔（秒）
heartbeatTimeout = 3          # 心跳超时次数
enableAck = true              # 是否启用消息确认
ackTimeout = 10               # 确认超时（秒）
```

**实现逻辑**:
```go
func (cm *ConnectionManager) CanAcceptConnection() bool {
    current := atomic.LoadInt32(&cm.currentConnections)
    if current >= cm.maxConnections {
        zap.L().Warn("WebSocket连接数已达上限", ...)
        return false
    }
    return true
}
```

#### 3. 消息确认机制设计

**消息格式扩展**:
```go
type ChatMessageRequest struct {
    // 原有字段...
    RequireAck bool   `json:"require_ack"` // 是否需要确认
    MessageID  string `json:"message_id"`   // 消息唯一ID
}

type AckMessage struct {
    Type      string `json:"type"`       // "ack"
    MessageID string `json:"message_id"` // 确认的消息ID
}
```

### 实施步骤

1. **Step 1**: 创建连接管理器 (30分钟)
   - 创建 `internal/service/chat/ws_connection.go`
   - 实现 `ConnectionManager` 结构体

2. **Step 2**: 修改配置系统 (20分钟)
   - 添加 WebSocketConfig
   - 更新 `configs/config.toml` 模板

3. **Step 3**: 集成到现有代码 (40分钟)
   - 修改 `ws_gateway.go:NewClientInit()`
   - 添加心跳检测和连接数检查

4. **Step 4**: 添加日志和监控 (20分钟)
   - 连接建立/断开日志
   - 心跳超时日志

### 预期效果

- ✅ 僵尸连接 90 秒内被清理
- ✅ 连接数可控，防止资源耗尽
- ✅ 重要消息可达率提升到 99.9%

---

## Phase 2: 数据库事务管理

### 当前状态：✅ 已完成

经审查，核心业务逻辑已正确使用事务包装。以下是已实现事务保护的关键操作：

| 操作 | 文件 | 方法 | 涉及表 |
|------|------|------|--------|
| 通过好友申请 | `apply/service.go` | `PassFriendApply` | Apply, Contact (x2) |
| 通过入群申请 | `apply/service.go` | `PassGroupApply` | Apply, Contact, GroupMember, Group |
| 免审核入群 | `apply/service.go` | `ApplyGroup` | GroupMember, Group, Contact, Apply |
| 创建群聊 | `group/service.go` | `CreateGroup` | Group, GroupMember, Contact, Session |
| 解散群聊 | `group/service.go` | `DismissGroup` | Group, GroupMember, Contact, Session, Apply |
| 退出群聊 | `group/service.go` | `LeaveGroup` | GroupMember, Group, Contact, Apply |
| 移除群成员 | `group/service.go` | `RemoveGroupMembers` | GroupMember, Group, Contact, Apply |

### 事务基础设施

**实现位置**: `internal/dao/mysql/repositories.go`

```go
func (r *Repositories) Transaction(fn func(txRepos *Repositories) error) error {
    return r.db.Transaction(func(tx *gorm.DB) error {
        return fn(NewRepositories(tx))
    })
}
```

**使用模式**:
```go
err := s.repos.Transaction(func(txRepos *mysql.Repositories) error {
    // 所有操作使用 txRepos 而非 s.repos
    if err := txRepos.Apply.Update(apply); err != nil {
        return err // 返回错误触发回滚
    }
    if err := txRepos.Contact.CreateContact(&contact); err != nil {
        return err
    }
    return nil // 返回 nil 提交事务
})
```

### 死锁预防策略

**场景**: 同时通过 A→B 和 B→A 的好友申请

**已采用方案**: 全局一致的表操作顺序
- 当前代码：总是先更新 Apply，再创建 Contact
- 确保多事务间的加锁顺序一致，避免循环等待

**可选增强**（若出现死锁）:
```go
// 按ID排序，保证全局一致的记录加锁顺序
ids := []string{userId, applicantId}
sort.Strings(ids)
for _, id := range ids {
    // 按排序后的顺序创建Contact
}
```

---

### 进阶优化建议

#### 1. 事务超时控制

**问题**: 长事务会阻塞其他操作，影响系统吞吐量

**方案**: 为事务设置上下文超时
```go
func (r *Repositories) TransactionWithTimeout(ctx context.Context, timeout time.Duration, 
    fn func(txRepos *Repositories) error) error {
    
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        return fn(NewRepositories(tx))
    })
}
```

**使用**:
```go
err := s.repos.TransactionWithTimeout(ctx, 5*time.Second, func(txRepos *mysql.Repositories) error {
    // ...
})
```

#### 2. 事务隔离级别

**GORM 默认**: `REPEATABLE READ` (MySQL 默认)

**特殊场景**: 若需要更高一致性（如金融场景），可指定 `SERIALIZABLE`
```go
db.Transaction(func(tx *gorm.DB) error {
    // ...
}, &sql.TxOptions{Isolation: sql.LevelSerializable})
```

> [!CAUTION]
> `SERIALIZABLE` 会显著降低并发性能，仅在绝对必要时使用。

#### 3. 重试机制

**适用场景**: 乐观锁冲突、死锁等可重试错误

```go
func RetryTransaction(repos *mysql.Repositories, maxRetries int, 
    fn func(txRepos *mysql.Repositories) error) error {
    
    var err error
    for i := 0; i < maxRetries; i++ {
        err = repos.Transaction(fn)
        if err == nil {
            return nil
        }
        // 检查是否为可重试错误
        if !isRetryableError(err) {
            return err
        }
        // 指数退避
        time.Sleep(time.Duration(1<<i) * 100 * time.Millisecond)
    }
    return fmt.Errorf("transaction failed after %d retries: %w", maxRetries, err)
}
```

---

### 最佳实践清单

- [x] **事务范围最小化**: 只在必要时开启事务，避免长事务
- [x] **统一错误处理**: 事务内返回 error 自动回滚
- [x] **缓存在事务外删除**: 事务成功后再异步清理缓存
- [ ] **添加事务超时**: 防止阻塞（待实施）
- [ ] **死锁监控**: 添加死锁告警（待实施）

---


## Phase 3: 配置管理强化

### 当前状态：✅ 已完成

配置管理已完成全面升级，包括线程安全、环境变量支持和健壮的错误处理。

### 已完成的改进

#### 1. 线程安全单例 (`sync.Once`)

**实现位置**: `internal/config/config.go`

```go
var (
    configInstance *Config
    once           sync.Once
)

func GetConfig() *Config {
    once.Do(func() {
        var err error
        configInstance, err = LoadConfig()
        if err != nil {
            panic(fmt.Sprintf("Failed to load configuration: %v", err))
        }
    })
    return configInstance
}
```

**效果**: 无论多少个 goroutine 同时调用 `GetConfig()`，配置只会被加载一次。

#### 2. 环境变量覆盖

**支持的环境变量**:
| 环境变量 | 覆盖的配置项 | 用途 |
|----------|--------------|------|
| `MYSQL_PASSWORD` | `mysqlConfig.password` | 数据库密码 |
| `REDIS_PASSWORD` | `redisConfig.password` | Redis 密码 |
| `JWT_SECRET` | `jwtConfig.secret` | JWT 签名密钥 |
| `KAFKA_HOST_PORT` | `kafkaConfig.hostPort` | Kafka 地址 |

**实现**:
```go
func overlayEnvVars(c *Config) {
    if v := os.Getenv("MYSQL_PASSWORD"); v != "" {
        c.MysqlConfig.Password = v
    }
    if v := os.Getenv("REDIS_PASSWORD"); v != "" {
        c.RedisConfig.Password = v
    }
    if v := os.Getenv("JWT_SECRET"); v != "" {
        c.JWTConfig.Secret = v
    }
    if v := os.Getenv("KAFKA_HOST_PORT"); v != "" {
        c.KafkaConfig.HostPort = v
    }
}
```

**使用示例** (Docker/云部署):
```bash
export MYSQL_PASSWORD="prod_secure_password"
export JWT_SECRET="super-long-production-secret-key-at-least-32-chars"
./kama_chat_server
```

#### 3. 健壮的路径解析

配置文件查找顺序：
1. `configs/config_local.toml` (本地开发优先)
2. `configs/config.toml`
3. `<可执行文件目录>/configs/config_local.toml`
4. `<可执行文件目录>/configs/config.toml`
5. `../../configs/config_local.toml` (从子目录运行)
6. `../../configs/config.toml`

#### 4. 严格错误处理

配置加载失败时，程序会立即 `panic` 而非继续运行：
```go
if err != nil {
    panic(fmt.Sprintf("Failed to load configuration: %v", err))
}
```

> [!IMPORTANT]
> 这是**有意设计**。运行时带着空配置会导致难以调试的零值错误（如连接 `localhost:0`）。快速失败更安全。

### 单元测试

**测试文件**: `internal/config/config_test.go`

```go
func TestLoadConfig_EnvOverlay(t *testing.T) {
    os.Setenv("MYSQL_PASSWORD", "env_secret_password")
    defer os.Unsetenv("MYSQL_PASSWORD")

    conf, err := LoadConfig()
    if err != nil {
        t.Fatalf("LoadConfig failed: %v", err)
    }

    if conf.MysqlConfig.Password != "env_secret_password" {
        t.Errorf("Expected MYSQL_PASSWORD to be overridden")
    }
}

func TestGetConfig_Singleton(t *testing.T) {
    c1 := GetConfig()
    c2 := GetConfig()
    if c1 != c2 {
        t.Error("GetConfig should return the same instance")
    }
}
```

**测试结果**: ✅ PASS

---

### 待办事项

- [ ] **添加配置验证函数**: 校验必填字段（如 JWT 密钥长度 ≥ 32）
- [ ] **添加更多环境变量**: 支持覆盖 MySQL Host/Port 等

---


## Phase 4: 代码重构

### 当前问题

#### 1. Token 生成逻辑重复
**位置**: `internal/service/user/service.go`
- Login 和 SmsLogin 方法中完全相同的 40 行 Token 代码

#### 2. 缓存操作模式重复
多处重复"查缓存→查库→回写缓存"逻辑

#### 3. 错误日志格式不统一
```go
// 格式1
zap.L().Error("service error", zap.Error(err))

// 格式2（中文）
zap.L().Error("创建消息失败", zap.Error(err))
```

#### 4. 调试代码未清理
**位置**: `internal/handler/user_handler.go:39`
```go
fmt.Println(req) // 调试输出
```

### 优化方案

#### 1. 提取 Token 生成逻辑

**新建文件**: `internal/service/helper/token_helper.go`

```go
package helper

type TokenHelper struct{}

func (h *TokenHelper) GenerateTokenPair(user *model.UserInfo) (*TokenPair, error) {
    accessToken, err := jwt.GenerateAccessToken(user.Uuid)
    if err != nil {
        return nil, err
    }

    refreshToken, tokenID, err := jwt.GenerateRefreshToken(user.Uuid)
    if err != nil {
        return nil, err
    }

    return &TokenPair{
        AccessToken:  accessToken,
        RefreshToken: refreshToken,
        TokenID:      tokenID,
    }, nil
}
```

#### 2. 统一缓存操作

**新建文件**: `internal/service/helper/cache_helper.go`

```go
func (h *CacheHelper) GetOrLoad(ctx context.Context, key string, 
    loader func() (interface{}, error), ttl int, result interface{}) error {
    
    // 1. 尝试从缓存获取
    if val, err := h.cache.GetOrError(ctx, key); err == nil {
        return json.Unmarshal([]byte(val), result)
    }
    
    // 2. 从 loader 加载
    data, err := loader()
    if err != nil {
        return err
    }
    
    // 3. 回写缓存
    if jsonData, err := json.Marshal(data); err == nil {
        h.cache.Set(ctx, key, string(jsonData), time.Duration(ttl)*time.Second)
    }
    
    return nil
}
```

#### 3. 统一错误日志

**新建文件**: `pkg/logutil/logutil.go`

```go
func ServiceError(operation string, err error, fields ...zap.Field) {
    allFields := append([]zap.Field{
        zap.String("layer", "service"),
        zap.String("operation", operation),
        zap.Error(err),
    }, fields...)
    zap.L().Error("service_error", allFields...)
}
```

---

## Phase 5: 缓存策略优化

### 当前状态：🟡 部分完成

缓存基础设施完善，但高级保护机制仍需加强。

### 已完成的基础设施

#### 1. 异步缓存任务队列

**实现位置**: `internal/dao/redis/impl.go`

```go
type RedisCache struct {
    client       *redis.Client
    taskChan     chan func()   // 异步任务通道
    workerNum    int           // Worker 数量
    taskChanSize int           // 缓冲区大小
}

// SubmitTask 提交异步缓存任务（非阻塞）
func (r *RedisCache) SubmitTask(action func()) {
    select {
    case r.taskChan <- action:
        // 成功放入队列
    default:
        // 降级：同步执行（通道满时）
        zap.L().Warn("Redis cache task channel full, executing synchronously")
        action()
    }
}
```

**优势**:
- 数据库写入后异步清理缓存，不阻塞主请求
- Worker Pool 模式，控制并发数
- 通道满时降级为同步执行，保证可靠性

#### 2. 接口隔离设计

```go
// CacheService: 基础同步读写（供 SmsService 等使用）
type CacheService interface {
    Set(ctx, key, value, ttl) error
    Get(ctx, key) (string, error)
    Delete(ctx, key) error
    // ...
}

// AsyncCacheService: 扩展异步任务（供 ChatServer 等使用）
type AsyncCacheService interface {
    CacheService
    SubmitTask(action func())
}
```

#### 3. 已实现的缓存模式

| 模式 | 使用场景 | 示例 |
|------|----------|------|
| Cache-Aside | 读取用户/群组信息 | `GetGroupDetail`, `GetUserInfo` |
| Write-Behind | 写入后异步删除缓存 | `UpdateGroupInfo`, `PassFriendApply` |
| Set 集合 | 存储联系人关系列表 | `contact_relation:user:xxx` |

---

### 当前问题与优化方案

#### 问题 1: 缓存穿透

**现象**: 查询不存在的数据时，每次都穿透到数据库

**优化方案**: 缓存空值标记

```go
func (s *userService) GetUserInfo(userId string) (*UserInfo, error) {
    cacheKey := "user_info_" + userId
    nullKey := cacheKey + ":null"
    
    // 1. 检查空值标记
    if val, _ := s.cache.Get(ctx, nullKey); val == "1" {
        return nil, errorx.New(errorx.CodeNotFound, "用户不存在")
    }
    
    // 2. 查缓存
    if data, err := s.cache.Get(ctx, cacheKey); err == nil && data != "" {
        // 反序列化返回
    }
    
    // 3. 查数据库
    user, err := s.repos.User.FindByUuid(userId)
    if err != nil {
        if errorx.IsNotFound(err) {
            // 缓存空值标记（5分钟）
            s.cache.Set(ctx, nullKey, "1", 5*time.Minute)
        }
        return nil, err
    }
    
    // 4. 回写缓存
    // ...
}
```

#### 问题 2: 缓存雪崩

**现象**: 大量热点 Key 同时过期，瞬间打爆数据库

**优化方案**: 随机过期时间

```go
// internal/dao/redis/cache/ttl.go
func RandomizedTTL(baseTTL time.Duration) time.Duration {
    // 添加 ±10% 的随机偏移
    jitter := time.Duration(rand.Int63n(int64(baseTTL) / 5))
    if rand.Intn(2) == 0 {
        return baseTTL + jitter
    }
    return baseTTL - jitter
}

// 使用
s.cache.Set(ctx, key, value, RandomizedTTL(24*time.Hour))
```

#### 问题 3: 热点 Key

**现象**: 单个 Key 请求量过高，压垮单个 Redis 节点

**优化方案**: 本地缓存 + 互斥锁

```go
// 本地缓存（进程内）
var localCache = sync.Map{}

func GetHotData(key string) (string, error) {
    // 1. 查本地缓存
    if val, ok := localCache.Load(key); ok {
        return val.(string), nil
    }
    
    // 2. 使用 singleflight 防止缓存击穿
    val, err, _ := singleflightGroup.Do(key, func() (interface{}, error) {
        // 查 Redis
        data, err := redis.Get(ctx, key)
        if err == nil {
            localCache.Store(key, data)
            // 设置本地缓存过期（10秒）
            go func() {
                time.Sleep(10 * time.Second)
                localCache.Delete(key)
            }()
        }
        return data, err
    })
    return val.(string), err
}
```

---

### 缓存 Key 命名规范

| 模式 | 格式 | 示例 |
|------|------|------|
| 用户信息 | `user_info_{userId}` | `user_info_U123456` |
| 群组信息 | `group_info_{groupId}` | `group_info_G789012` |
| 成员列表 | `group_memberlist_{groupId}` | `group_memberlist_G789012` |
| 联系人关系 | `contact_relation:{type}:{userId}` | `contact_relation:user:U123456` |
| 会话列表 | `{type}_session_list_{userId}*` | `group_session_list_U123456*` |
| 空值标记 | `{key}:null` | `user_info_U999:null` |

---

### 实施清单

- [x] 异步缓存任务队列
- [x] 接口隔离设计
- [x] Cache-Aside 读写模式
- [x] 缓存空值标记（防穿透）— `internal/dao/redis/cache/helper.go`
- [x] 随机过期时间（防雪崩）— `internal/dao/redis/cache/ttl.go`
- [ ] 热点 Key 本地缓存（按需实施）
- [x] singleflight 防击穿 — `internal/dao/redis/cache/helper.go`

---


## Phase 6: 限流保护

### API 限流

**新建文件**: `internal/infrastructure/middleware/ratelimit.go`

```go
package middleware

import (
    "net/http"
    "sync"
    "golang.org/x/time/rate"
    "github.com/gin-gonic/gin"
)

type RateLimiter struct {
    limiters map[string]*rate.Limiter
    mu       sync.RWMutex
    r        rate.Limit
    b        int
}

func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
    return &RateLimiter{
        limiters: make(map[string]*rate.Limiter),
        r:        r,
        b:        b,
    }
}

func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
    return func(c *gin.Context) {
        key := c.ClientIP()
        if userID, exists := c.Get("user_id"); exists {
            key = key + "_" + userID.(string)
        }
        
        lim := limiter.GetLimiter(key)
        if !lim.Allow() {
            c.JSON(http.StatusTooManyRequests, gin.H{
                "code": 1009,
                "msg":  "请求过于频繁，请稍后重试",
                "data": nil,
            })
            c.Abort()
            return
        }
        
        c.Next()
    }
}
```

**使用方式**:
```go
limiter := middleware.NewRateLimiter(100, 200)
router.Use(middleware.RateLimitMiddleware(limiter))
```

### WebSocket 限流

```go
type UserConn struct {
    // ... 原有字段
    messageLimiter *rate.Limiter
    lastMessageTime time.Time
    messageCount    int
}

func (c *UserConn) Read() {
    for {
        _, jsonMessage, err := c.Conn.ReadMessage()
        if err != nil {
            return
        }
        
        // 限流检查
        if !c.messageLimiter.Allow() {
            zap.L().Warn("WebSocket消息限流", zap.String("user_id", c.Uuid))
            continue
        }
        
        // Flood 检测（10秒内超过100条）
        now := time.Now()
        if now.Sub(c.lastMessageTime) > 10*time.Second {
            c.messageCount = 0
            c.lastMessageTime = now
        }
        c.messageCount++
        if c.messageCount > 100 {
            zap.L().Error("WebSocket Flood攻击检测", zap.String("user_id", c.Uuid))
            c.Conn.Close()
            return
        }
    }
}
```

---

## Phase 7: 可维护性改进

### 1. 缓存 Key 统一管理

**新建文件**: `pkg/constants/cache_keys.go`

```go
package constants

const (
    UserInfoKey       = "user_info_%s"
    MessageListKey    = "message_list_%s_%s"
    GroupMessageKey   = "group_messagelist_%s"
)

func BuildUserInfoKey(userID string) string {
    return fmt.Sprintf(UserInfoKey, userID)
}
```

### 2. 添加单元测试框架

**新建文件**: `internal/service/user/service_test.go`

```go
package user

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestUserService_Login_Success(t *testing.T) {
    mockRepo := new(MockUserRepository)
    mockCache := new(MockCacheService)
    
    user := &model.UserInfo{
        Uuid:     "U123456789",
        Nickname: "TestUser",
    }
    
    mockRepo.On("FindByTelephone", "13800138000").Return(user, nil)
    
    svc := &userInfoService{
        repos: &mysql.Repositories{User: mockRepo},
        cache: mockCache,
    }
    
    req := auth.LoginRequest{
        Telephone: "13800138000",
        Password:  "correct_password",
    }
    
    result, err := svc.Login(req)
    
    assert.NoError(t, err)
    assert.NotNil(t, result)
    mockRepo.AssertExpectations(t)
}
```

---

## 实施建议

### 推荐顺序

**保守方案**（逐步验证）：
1. Phase 3 → 2 → 1 → 4 → 5 → 6 → 7
2. 每完成一个阶段进行测试验证

**激进方案**（快速上线）：
1. Phase 1 + 2 + 3 并行开发（核心稳定性）
2. Phase 4 + 5 + 6 后续跟进（性能和安全）
3. Phase 7 长期维护

### 依赖关系

```
Phase 3 (配置) ──┬──► Phase 1 (WebSocket配置依赖)
                 └──► Phase 2 (无需依赖)

Phase 1 ────────┐
Phase 2 ────────┼──► Phase 6 (限流需要连接管理)
                │
Phase 4 ────────┼──► Phase 5 (缓存重构后优化)
                │
Phase 5 ────────┘
```

---

## 预期效果

### 稳定性提升
- ✅ 僵尸连接 90 秒内自动清理
- ✅ 连接数可控，防止资源耗尽
- ✅ 数据一致性 100% 保证
- ✅ 无效配置无法启动

### 性能优化
- ✅ 缓存命中率提升
- ✅ 减少重复数据库查询
- ✅ 合理的限流保护

### 可维护性
- ✅ 代码重复率降低 50%
- ✅ 统一的错误日志格式
- ✅ 完善的配置验证
- ✅ 单元测试框架搭建

### 安全性
- ✅ API 限流防止攻击
- ✅ WebSocket Flood 检测
- ✅ 配置敏感项验证

---

## 风险评估

| 风险 | 阶段 | 可能性 | 缓解措施 |
|------|------|--------|----------|
| 心跳增加CPU负载 | 1 | 低 | 30秒间隔足够宽松 |
| 事务降低吞吐量 | 2 | 中 | 仅关键业务使用 |
| 配置验证太严格 | 3 | 低 | 提供合理默认值 |
| 限流误伤正常用户 | 6 | 中 | 阈值可调，监控告警 |
| 重构引入Bug | 4 | 中 | 保留原代码，逐步替换 |

---

## 回滚策略

每个阶段独立实施，出现问题可单独回滚：

1. **保留原代码分支**: `git checkout -b backup/pre-optimization`
2. **阶段性提交**: 每个阶段独立 commit
3. **快速回滚**: `git revert <commit-hash>`
4. **配置兼容**: 新配置项提供默认值，旧版本也能运行

---

## 监控指标

优化后需要关注的指标：

### WebSocket 指标
- `ws_connections_active`: 当前连接数
- `ws_heartbeat_miss`: 心跳超时次数
- `ws_message_ack_rate`: 消息确认率

### 数据库指标
- `db_transaction_duration`: 事务执行时间
- `db_deadlock_count`: 死锁次数

### 缓存指标
- `cache_hit_rate`: 缓存命中率
- `cache_stale_data_count`: 脏数据次数

### 限流指标
- `rate_limit_hit_count`: 限流触发次数
- `ws_flood_detected`: Flood 攻击检测次数

---

## 下一步行动

### 立即执行
1. 创建优化分支: `git checkout -b feature/architecture-optimization`
2. 从 Phase 1 开始实施
3. 每个阶段完成后运行测试

### 需要确认
- [ ] WebSocket 心跳间隔（建议30秒）
- [ ] 最大连接数限制（建议10000）
- [ ] API 限流阈值（建议100/分钟）
- [ ] 实施顺序（保守或激进）

---

**文档版本**: v1.0  
**最后更新**: 2026-01-31
