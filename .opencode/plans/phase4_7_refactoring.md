# Phase 4-7: 代码重构与可维护性优化方案

## Phase 4: 代码重构 - 减少重复代码

### 当前问题

#### 1. Token 生成逻辑重复
**位置**: `internal/service/user/service.go`
```go
// Login 方法中
accessToken, err := jwt.GenerateAccessToken(user.Uuid)
// ... 约40行 Token 相关代码

// SmsLogin 方法中
accessToken, err := jwt.GenerateAccessToken(user.Uuid)
// ... 完全相同的40行代码
```

#### 2. 缓存操作模式重复
**多处出现**:
```go
// 查缓存
val, err := cache.Get()
if err != nil {
    // 查数据库
    data, err := db.Query()
    if err != nil {
        return err
    }
    // 回写缓存
    cache.Set(data)
    return data
}
return val
```

#### 3. 错误日志格式不统一
**多处出现**:
```go
// 格式1
zap.L().Error("service error", zap.Error(err))

// 格式2
zap.L().Error("数据库错误", zap.Error(err), zap.String("sql", sql))

// 格式3（中文）
zap.L().Error("创建消息失败", zap.Error(err))
```

#### 4. 调试代码未清理
**位置**: `internal/handler/user_handler.go:39`
```go
fmt.Println(req) // 调试输出，生产环境可删除
```

---

### 优化方案

#### 1. 提取 Token 生成逻辑

**新建文件**: `internal/service/helper/token_helper.go`

```go
package helper

import (
    "kama_chat_server/internal/model"
    "kama_chat_server/pkg/util/jwt"
    userrsp "kama_chat_server/internal/dto/respond/user"
    "go.uber.org/zap"
)

// TokenPair Token 对
type TokenPair struct {
    AccessToken  string
    RefreshToken string
    TokenID      string
}

// TokenHelper Token 生成辅助函数
type TokenHelper struct{}

// NewTokenHelper 创建 TokenHelper
func NewTokenHelper() *TokenHelper {
    return &TokenHelper{}
}

// GenerateTokenPair 生成双 Token
func (h *TokenHelper) GenerateTokenPair(user *model.UserInfo) (*TokenPair, error) {
    accessToken, err := jwt.GenerateAccessToken(user.Uuid)
    if err != nil {
        zap.L().Error("生成 Access Token 失败", zap.Error(err))
        return nil, err
    }

    refreshToken, tokenID, err := jwt.GenerateRefreshToken(user.Uuid)
    if err != nil {
        zap.L().Error("生成 Refresh Token 失败", zap.Error(err))
        return nil, err
    }

    return &TokenPair{
        AccessToken:  accessToken,
        RefreshToken: refreshToken,
        TokenID:      tokenID,
    }, nil
}

// BuildLoginResponse 构建登录响应
func (h *TokenHelper) BuildLoginResponse(user *model.UserInfo, tokens *TokenPair) *userrsp.LoginRespond {
    return &userrsp.LoginRespond{
        AccessToken:  tokens.AccessToken,
        RefreshToken: tokens.RefreshToken,
        UserInfo: userrsp.UserInfoRespond{
            Uuid:       user.Uuid,
            Nickname:   user.Nickname,
            Telephone:  user.Telephone,
            Email:      user.Email,
            Avatar:     user.Avatar,
            Gender:     user.Gender,
            Signature:  user.Signature,
            Birthday:   user.Birthday,
            Status:     user.Status,
            IsAdmin:    user.IsAdmin,
        },
    }
}
```

**使用方式**:
```go
// 在 user/service.go 中
tokens, err := h.tokenHelper.GenerateTokenPair(user)
if err != nil {
    return nil, errorx.ErrServerBusy
}

return h.tokenHelper.BuildLoginResponse(user, tokens), nil
```

#### 2. 统一缓存操作

**新建文件**: `internal/service/helper/cache_helper.go`

```go
package helper

import (
    "context"
    "encoding/json"
    "time"
    
    myredis "kama_chat_server/internal/dao/redis"
    "kama_chat_server/pkg/constants"
    "go.uber.org/zap"
)

// CacheHelper 缓存辅助函数
type CacheHelper struct {
    cache myredis.AsyncCacheService
}

// NewCacheHelper 创建 CacheHelper
func NewCacheHelper(cache myredis.AsyncCacheService) *CacheHelper {
    return &CacheHelper{cache: cache}
}

// GetOrLoad 从缓存获取，如果不存在则从 loader 加载并缓存
// key: 缓存键
// loader: 数据加载函数
// ttl: 缓存过期时间（秒）
// result: 结果指针
func (h *CacheHelper) GetOrLoad(ctx context.Context, key string, 
    loader func() (interface{}, error), ttl int, result interface{}) error {
    
    // 1. 尝试从缓存获取
    if h.cache != nil {
        val, err := h.cache.GetOrError(ctx, key)
        if err == nil {
            // 缓存命中，反序列化
            if err := json.Unmarshal([]byte(val), result); err == nil {
                return nil
            }
            // 反序列化失败，继续查库
        }
    }
    
    // 2. 缓存未命中或出错，从 loader 加载
    data, err := loader()
    if err != nil {
        return err
    }
    
    // 3. 回写缓存（异步）
    if h.cache != nil {
        h.cache.SubmitTask(func() {
            if jsonData, err := json.Marshal(data); err == nil {
                _ = h.cache.Set(ctx, key, string(jsonData), time.Duration(ttl)*time.Second)
            }
        })
    }
    
    // 4. 设置结果
    resultData, _ := json.Marshal(data)
    return json.Unmarshal(resultData, result)
}

// DeleteCache 删除缓存
func (h *CacheHelper) DeleteCache(ctx context.Context, key string) {
    if h.cache != nil {
        h.cache.SubmitTask(func() {
            _ = h.cache.Delete(ctx, key)
        })
    }
}

// DeleteCacheSync 同步删除缓存（用于关键数据）
func (h *CacheHelper) DeleteCacheSync(ctx context.Context, key string) {
    if h.cache != nil {
        _ = h.cache.Delete(ctx, key)
    }
}
```

**使用方式**:
```go
// 改造前的代码
val, err := cache.Get()
if err != nil {
    data, err := db.Query()
    if err != nil {
        return err
    }
    cache.Set(data)
    return data
}
return val

// 改造后的代码
var result UserInfo
err := cacheHelper.GetOrLoad(ctx, "user_"+userId, 
    func() (interface{}, error) {
        return repos.User.FindByUuid(userId)
    }, 
    constants.REDIS_TIMEOUT, 
    &result,
)
```

#### 3. 统一错误日志

**新建文件**: `pkg/logutil/logutil.go`

```go
package logutil

import (
    "go.uber.org/zap"
)

// ServiceError 记录 Service 层错误
func ServiceError(operation string, err error, fields ...zap.Field) {
    allFields := append([]zap.Field{
        zap.String("layer", "service"),
        zap.String("operation", operation),
        zap.Error(err),
    }, fields...)
    zap.L().Error("service_error", allFields...)
}

// DBError 记录数据库错误
func DBError(operation string, err error, fields ...zap.Field) {
    allFields := append([]zap.Field{
        zap.String("layer", "dao"),
        zap.String("operation", operation),
        zap.Error(err),
    }, fields...)
    zap.L().Error("db_error", allFields...)
}

// CacheError 记录缓存错误
func CacheError(operation string, err error, fields ...zap.Field) {
    allFields := append([]zap.Field{
        zap.String("layer", "cache"),
        zap.String("operation", operation),
        zap.Error(err),
    }, fields...)
    zap.L().Error("cache_error", allFields...)
}

// WSError 记录 WebSocket 错误
func WSError(operation string, err error, fields ...zap.Field) {
    allFields := append([]zap.Field{
        zap.String("layer", "websocket"),
        zap.String("operation", operation),
        zap.Error(err),
    }, fields...)
    zap.L().Error("ws_error", allFields...)
}

// ServiceWarn 记录 Service 层警告
func ServiceWarn(operation string, message string, fields ...zap.Field) {
    allFields := append([]zap.Field{
        zap.String("layer", "service"),
        zap.String("operation", operation),
        zap.String("message", message),
    }, fields...)
    zap.L().Warn("service_warn", allFields...)
}
```

**使用方式**:
```go
// 改造前
zap.L().Error("service error", zap.Error(err))

// 改造后
logutil.ServiceError("UpdateUserInfo", err, zap.String("user_id", userId))
```

#### 4. 清理调试代码

**修改文件**: `internal/handler/user_handler.go:39`

```go
// 删除这行
// fmt.Println(req) // 调试输出，生产环境可删除
```

---

## Phase 5: 缓存策略优化

### 当前问题

#### 1. 缓存一致性问题
**位置**: `internal/service/user/service.go`
```go
// 更新数据库
if err := u.repos.User.UpdateUserInfo(user); err != nil {
    return errorx.ErrServerBusy
}
// 异步清理缓存（可能延迟）
u.cache.SubmitTask(func() {
    u.cache.Delete(context.Background(), "user_info_"+userId)
})
```
**问题**: 异步删除有延迟，期间可能读取旧数据

#### 2. 缓存穿透
**问题**: 查询不存在的数据时，每次都查库

#### 3. 缓存雪崩
**问题**: 大量热点 Key 同时过期

---

### 优化方案

#### 1. 关键数据同步删除

```go
// 对于关键数据（如用户信息），使用同步删除
func (s *userService) UpdateUserInfo(userId string, req UpdateRequest) error {
    // 1. 更新数据库
    if err := s.repos.User.Update(userId, req); err != nil {
        return err
    }
    
    // 2. 同步删除缓存（关键数据不能延迟）
    s.cacheHelper.DeleteCacheSync(ctx, "user_info_"+userId)
    
    return nil
}
```

#### 2. 缓存空值

```go
func (h *CacheHelper) GetOrLoadWithNullCache(ctx context.Context, key string, 
    loader func() (interface{}, error), ttl int, nullTTL int, result interface{}) error {
    
    // 检查空值缓存标记
    nullKey := key + ":null"
    if val, _ := h.cache.GetOrError(ctx, nullKey); val == "1" {
        return errorx.New(errorx.CodeNotFound, "数据不存在")
    }
    
    // 正常查询
    data, err := loader()
    if err != nil {
        if errorx.IsNotFound(err) {
            // 缓存空值标记（5分钟）
            h.cache.Set(ctx, nullKey, "1", time.Duration(nullTTL)*time.Second)
        }
        return err
    }
    
    // 缓存数据
    // ...
    return nil
}
```

#### 3. 随机过期时间

```go
func RandomizedTTL(baseTTL int) time.Duration {
    // 添加 ±10% 的随机偏移
    offset := rand.Intn(baseTTL*2/10) - baseTTL/10
    return time.Duration(baseTTL+offset) * time.Second
}

// 使用
_ = h.cache.Set(ctx, key, string(jsonData), RandomizedTTL(constants.REDIS_TIMEOUT))
```

---

## Phase 6: 限流保护

### API 限流

**新建文件**: `internal/infrastructure/middleware/ratelimit.go`

```go
package middleware

import (
    "net/http"
    "sync"
    "time"
    
    "github.com/gin-gonic/gin"
    "golang.org/x/time/rate"
)

// RateLimiter 限流器
type RateLimiter struct {
    limiters map[string]*rate.Limiter
    mu       sync.RWMutex
    r        rate.Limit // 每秒产生的令牌数
    b        int        // 桶大小
}

// NewRateLimiter 创建限流器
// r: 每秒产生的令牌数（如 100）
// b: 桶大小（如 200）
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
    return &RateLimiter{
        limiters: make(map[string]*rate.Limiter),
        r:        r,
        b:        b,
    }
}

// GetLimiter 获取限流器
func (rl *RateLimiter) GetLimiter(key string) *rate.Limiter {
    rl.mu.RLock()
    limiter, exists := rl.limiters[key]
    rl.mu.RUnlock()
    
    if !exists {
        rl.mu.Lock()
        limiter, exists = rl.limiters[key]
        if !exists {
            limiter = rate.NewLimiter(rl.r, rl.b)
            rl.limiters[key] = limiter
        }
        rl.mu.Unlock()
    }
    
    return limiter
}

// RateLimitMiddleware API 限流中间件
func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 使用 IP + UserID 作为限流键
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
// router.go 中添加
limiter := middleware.NewRateLimiter(100, 200) // 100/s, burst 200
router.Use(middleware.RateLimitMiddleware(limiter))
```

### WebSocket 限流

```go
// UserConn 添加限流字段
type UserConn struct {
    // ... 原有字段
    messageLimiter *rate.Limiter // 消息发送限流
    lastMessageTime time.Time    // 上次消息时间
    messageCount    int          // 消息计数（用于短时间检测）
}

// 在 Read() 中添加限流检查
func (c *UserConn) Read() {
    for {
        _, jsonMessage, err := c.Conn.ReadMessage()
        if err != nil {
            return
        }
        
        // 限流检查
        if !c.messageLimiter.Allow() {
            zap.L().Warn("WebSocket消息限流", zap.String("user_id", c.Uuid))
            // 发送限流提示
            c.SendBack <- &MessageBack{
                Message: []byte(`{"type":"error","msg":"消息发送过于频繁"}`),
            }
            continue
        }
        
        //  Flood 检测（10秒内超过100条）
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
        
        // 处理消息...
    }
}
```

---

## Phase 7: 可维护性改进

### 1. 缓存 Key 统一管理

**新建文件**: `pkg/constants/cache_keys.go`

```go
package constants

// 用户相关缓存键
const (
    UserInfoKey       = "user_info_%s"       // 用户信息
    UserSessionKey    = "user_session_%s"    // 用户会话列表
)

// 消息相关缓存键
const (
    MessageListKey    = "message_list_%s_%s" // 单聊消息列表
    GroupMessageKey   = "group_messagelist_%s" // 群聊消息列表
)

// 群组相关缓存键
const (
    GroupInfoKey      = "group_info_%s"      // 群组信息
    GroupMemberKey    = "group_members_%s"   // 群成员列表
)

// 构建缓存键的辅助函数
func BuildUserInfoKey(userID string) string {
    return fmt.Sprintf(UserInfoKey, userID)
}

func BuildMessageListKey(user1, user2 string) string {
    // 确保顺序一致
    if user1 > user2 {
        user1, user2 = user2, user1
    }
    return fmt.Sprintf(MessageListKey, user1, user2)
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

// MockUserRepository Mock 用户仓库
type MockUserRepository struct {
    mock.Mock
}

func (m *MockUserRepository) FindByUuid(uuid string) (*model.UserInfo, error) {
    args := m.Called(uuid)
    return args.Get(0).(*model.UserInfo), args.Error(1)
}

// ... 其他 Mock 方法

func TestUserService_Login_Success(t *testing.T) {
    // 准备 Mock
    mockRepo := new(MockUserRepository)
    mockCache := new(MockCacheService)
    
    user := &model.UserInfo{
        Uuid:     "U123456789",
        Nickname: "TestUser",
        Password: "$2a$10$...", // bcrypt hash
    }
    
    mockRepo.On("FindByTelephone", "13800138000").Return(user, nil)
    
    // 创建 Service
    svc := &userInfoService{
        repos: &mysql.Repositories{User: mockRepo},
        cache: mockCache,
    }
    
    // 执行测试
    req := auth.LoginRequest{
        Telephone: "13800138000",
        Password:  "correct_password",
    }
    
    result, err := svc.Login(req)
    
    // 验证结果
    assert.NoError(t, err)
    assert.NotNil(t, result)
    assert.NotEmpty(t, result.AccessToken)
    mockRepo.AssertExpectations(t)
}

func TestUserService_Login_InvalidPassword(t *testing.T) {
    // 测试密码错误场景
    // ...
}
```

### 3. 目录结构优化

```
internal/
├── service/
│   ├── interfaces.go          # Service 接口定义
│   ├── provider.go            # Service 依赖注入
│   └── user/
│       ├── service.go         # 主实现
│       ├── service_test.go    # 单元测试
│       └── helper.go          # 私有辅助函数
```

---

## 实施建议

### 优先级
1. **Phase 4**: 代码重构（提高可维护性）
2. **Phase 5**: 缓存优化（提升稳定性）
3. **Phase 6**: 限流保护（安全加固）
4. **Phase 7**: 测试和文档（长期收益）

### 时间估算
- Phase 4: 3-4 小时
- Phase 5: 2-3 小时
- Phase 6: 2-3 小时
- Phase 7: 3-4 小时

**总计**: 10-14 小时
