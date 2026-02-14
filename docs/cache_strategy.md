、】】【【【【【
、
# KamaChat 缓存策略优化方案

## 1. 概述

本文档描述了 KamaChat 应用程序中实施的缓存策略优化，旨在解决三大缓存问题：缓存穿透、缓存击穿和缓存雪崩。

### 1.1 问题背景

| 问题 | 描述 | 影响 |
|------|------|------|
| 缓存穿透 | 查询不存在的数据时，每次都穿透到数据库 | 数据库压力增大 |
| 缓存击穿 | 热点 Key 过期时，大量请求同时查询数据库 | 瞬时数据库负载过高 |
| 缓存雪崩 | 大量 Key 同时过期，导致数据库被打爆 | 系统可能崩溃 |

## 2. 解决方案

### 2.1 新增工具包

**路径**: `pkg/util/cache/`

#### 2.1.1 RandomizedTTL（防雪崩）

```go
// 返回带 ±10% 随机偏移的过期时间
func RandomizedTTL(baseTTL time.Duration) time.Duration

// 自定义抖动百分比
func TTLWithJitter(baseTTL time.Duration, jitterPercent int) time.Duration
```

**原理**: 通过给缓存过期时间添加随机抖动，避免大量 Key 在同一时刻过期。

#### 2.1.2 Cache Helper（防穿透 + 防击穿）

```go
type Helper struct {
    cache myredis.CacheService
    sf    singleflight.Group  // 防止缓存击穿
}

func (h *Helper) GetOrLoad(
    ctx context.Context,
    key string,
    loader func() (interface{}, error),  // 数据加载函数
    ttl time.Duration,                    // 正常数据 TTL
    nullTTL time.Duration,                // 空值 TTL（防穿透）
    result interface{},
) error
```

**特性**:
- **singleflight**: 同一时刻对同一 Key 的多个请求，只会有一个真正查询数据库
- **空值缓存**: 对于不存在的数据，缓存一个空值标记（如 `key:null = "1"`），避免重复查询

## 3. 集成的服务

| 服务文件 | 优化方法 | 缓存 Key |
|----------|----------|----------|
| `user/service.go` | `GetUserInfo` | `user_info_{userId}` |
| `user/service.go` | `GetPublicUserInfo` | `public_user_info_{userId}` |
| `group/service.go` | `GetPublicGroupInfo` | `group_info_{groupId}` |
| `contact/service.go` | `GetFriendInfo` | `user_info_{friendId}` |
| `contact/service.go` | `GetGroupDetail` | `group_info_{groupId}` |

## 4. 使用示例

### 4.1 基本用法

```go
// 在 Service 结构体中添加 cacheHelper
type userInfoService struct {
    repos       *mysql.Repositories
    cache       myredis.AsyncCacheService
    cacheHelper *cacheutil.Helper  // 新增
}

// 构造函数中初始化
func NewUserService(...) *userInfoService {
    return &userInfoService{
        repos:       repos,
        cache:       cacheService,
        cacheHelper: cacheutil.NewHelper(cacheService),
    }
}

// 在查询方法中使用
func (s *userInfoService) GetUserInfo(userId string) (*UserInfo, error) {
    var result UserInfo
    err := s.cacheHelper.GetOrLoad(
        context.Background(),
        "user_info_"+userId,
        func() (interface{}, error) {
            return s.repos.User.FindByUuid(userId)
        },
        cacheutil.RandomizedTTL(time.Hour),  // 1小时 ± 10%
        5*time.Minute,                        // 空值缓存 5 分钟
        &result,
    )
    return &result, err
}
```

## 5. 缓存 Key 命名规范

| 类型 | 格式 | 示例 |
|------|------|------|
| 用户信息 | `user_info_{userId}` | `user_info_U123456` |
| 用户公开信息 | `public_user_info_{userId}` | `public_user_info_U123456` |
| 群组信息 | `group_info_{groupId}` | `group_info_G789012` |
| 成员列表 | `group_memberlist_{groupId}` | `group_memberlist_G789012` |
| 联系人关系 | `contact_relation:{type}:{userId}` | `contact_relation:user:U123456` |
| 空值标记 | `{key}:null` | `user_info_U999:null` |

## 6. 单元测试

**测试文件**: `pkg/util/cache/ttl_test.go`

```
=== RUN   TestRandomizedTTL
--- PASS: TestRandomizedTTL (0.00s)
=== RUN   TestRandomizedTTL_ZeroValue
--- PASS: TestRandomizedTTL_ZeroValue (0.00s)
=== RUN   TestRandomizedTTL_SmallValue
--- PASS: TestRandomizedTTL_SmallValue (0.00s)
=== RUN   TestTTLWithJitter
--- PASS: TestTTLWithJitter (0.00s)
=== RUN   TestTTLWithJitter_ZeroJitter
--- PASS: TestTTLWithJitter_ZeroJitter (0.00s)
PASS
```

## 7. 总结

本次优化通过以下机制提升了系统的缓存可靠性：

1. **RandomizedTTL**: 防止缓存雪崩
2. **singleflight**: 防止缓存击穿
3. **空值缓存**: 防止缓存穿透

已集成到 `user`、`group`、`contact` 三个核心服务中，所有更改已通过编译验证并推送到代码仓库。
