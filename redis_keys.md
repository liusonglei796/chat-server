# Redis 键值文档

本文档列出了 KamaChat 应用中所有 Redis 键值，按模块分类。

常量定义位置：`pkg/constants/constants.go`

---

## 1. 认证与用户

| 键模式 | 常量名 | 类型 | TTL | 说明 | 读取位置 | 写入位置 | 删除位置 |
|--------|--------|------|-----|------|----------|----------|----------|
| `user:token:{userId}` | `CacheKeyUserToken` | String | 168 小时（7 天） | 存储 Refresh Token ID，用于单点登录互踢 | `auth/service.go:36` | `user/service.go:95` | — |
| `user:info:{userId}` | `CacheKeyUserInfo` | String (JSON) | RandomizedTTL | 完整用户信息（`GetUserInfoRespond`） | `user/service.go:313`、`friendship/service.go:87,134`、`session/service.go:271` | `cache/helper.go`（Cache-Aside 自动回写） | `user/service.go:293`、`admin/user/service.go:75,104,144,178` |
| `user:public_info:{userId}` | `CacheKeyUserPublicInfo` | String (JSON) | RandomizedTTL | 公开用户信息（`PublicUserInfoRespond`） | `user/service.go:354` | `cache/helper.go`（Cache-Aside 自动回写） | `user/service.go:297` |
| `auth:code:{telephone}` | `CacheKeyAuthCode` | String | 1 分钟 | 短信验证码 | `user/service.go:151,190` | `user/service.go:151,190`（读取校验） | `user/service.go:160,199` |

---

## 2. 好友关系

| 键模式 | 常量名 | 类型 | TTL | 说明 | 读取位置 | 写入位置 | 删除位置 |
|--------|--------|------|-----|------|----------|----------|----------|
| `friend_relation:user:{userId}` | `CacheKeyFriendRelUser` | Set | 无（持久） | 用户的好友 UUID 集合 | Cache-Aside 读取 | `friendship/service.go:327,328`（AddToSet） | `friendship/service.go:57,58`（RemoveFromSet）、`apply/service.go:563,564`（DeleteByPattern）、`admin/user/service.go:147`（DeleteByPatterns） |

---

## 3. 群组信息

| 键模式 | 常量名 | 类型 | TTL | 说明 | 读取位置 | 写入位置 | 删除位置 |
|--------|--------|------|-----|------|----------|----------|----------|
| `group:info:{groupId}` | `CacheKeyGroupInfo` | String (JSON) | RandomizedTTL | 群组详情（`GetGroupInfoRespond`） | `group/service.go:171`、`session/service.go:305` | `cache/helper.go`（Cache-Aside 自动回写） | `group/service.go:256,335,411,539`、`apply/service.go:270,670`、`admin/group/service.go:119,165`（InvalidateWithNull） |
| `group:members:{groupId}` | `CacheKeyGroupMembers` | String (JSON) | RandomizedTTL | 群成员详情列表（`[]GetGroupMemberListRespond`） | Cache-Aside 读取 | `cache/helper.go`（Cache-Aside 自动回写） | `group/service.go:259,337,414,541`、`apply/service.go:271,671`、`admin/group/service.go:122`（Delete） |
| `group:member_ids:{groupId}` | `CacheKeyGroupMemberIDs` | String (JSON) | 5 分钟 | 群成员 ID 列表（`[]string`），Kafka 消费者消息分发时使用 | `kafka_broker.go:675` | `kafka_broker.go:702` | — |

---

## 4. 会话

| 键模式 | 常量名 | 类型 | TTL | 说明 | 读取位置 | 写入位置 | 删除位置 |
|--------|--------|------|-----|------|----------|----------|----------|
| `session:open:{sendId}_{receiveId}` | `CacheKeySessionOpen` | String (JSON) | 1 分钟 | 单个会话缓存（`model.Session`） | `session/service.go:347` | `session/service.go:371` | — |
| `session:direct:{userId}*` | `CacheKeySessionDirect` | — | — | 私聊会话列表（仅用于 DeleteByPattern 清理） | — | — | `session/service.go:201`、`friendship/service.go:59,60`、`admin/user/service.go:105,145` |
| `session:group:{userId}*` | `CacheKeySessionGroup` | — | — | 群聊会话列表（仅用于 DeleteByPattern 清理） | — | — | `session/service.go:198`、`group/service.go:90,253,328,407,534`、`apply/service.go:269,669`、`admin/group/service.go:112`、`admin/user/service.go:106,146` |

---

## 5. 消息

| 键模式 | 常量名 | 类型 | TTL | 说明 | 读取位置 | 写入位置 | 删除位置 |
|--------|--------|------|-----|------|----------|----------|----------|
| `message:list:{id1}_{id2}` | `CacheKeyMessageList` | String (JSON) | 1 分钟 | 私聊消息列表（`[]GetMessageListRespond`），ID 按字典序排列 | `kafka_broker.go:579`（读取后追加） | `kafka_broker.go:585`（追加新消息后回写） | — |
| `message:group_list:{groupId}` | `CacheKeyGroupMessageList` | String (JSON) | 1 分钟 | 群聊消息列表（`[]GetMessageListRespond`） | `kafka_broker.go:654`（读取后追加） | `kafka_broker.go:660`（追加新消息后回写） | — |

---

## 6. 限流（中间件层）

限流 Key 不在 `constants.go` 中定义，而是在路由注册时作为参数传入 `RateLimit()` 中间件。

| 键模式 | 类型 | TTL（窗口） | 说明 | 定义位置 |
|--------|------|-------------|------|----------|
| `rate:login:{clientIP}` | Counter (INCR) | 5 分钟 | 登录限流，同一 IP 每 5 分钟最多 10 次 | `auth_routes.go:19` |
| `rate:sms:{telephone}` | Counter (INCR) | 60 秒 | 短信限流，同一手机号每 60 秒最多 1 次 | `auth_routes.go:24` |

中间件实现：`middleware/rate_limit.go` — 使用 `INCR` 原子递增 + `EXPIRE` 设置窗口过期。

---

## 7. SMS 验证码

SMS 服务与 User Service 统一使用 `constants.CacheKeyAuthCode`（`"auth:code:"`）前缀。

| 键模式 | 常量名 | 类型 | TTL | 说明 | 读取位置 | 写入位置 | 删除位置 |
|--------|--------|------|-----|------|----------|----------|----------|
| `auth:code:{telephone}` | `CacheKeyAuthCode` | String | 1 分钟 | 短信验证码（频率限制 + 验证码存储） | `sms/auth_code_service.go:37,124`（频率检查）、`user/service.go:151,190`（校验） | `sms/auth_code_service.go:50,143` | `user/service.go:160,199`、`sms/auth_code_service.go:178`（发送失败回滚） |

---

## 缓存策略说明

### Cache-Aside 模式
用户信息、群组信息等读多写少的数据采用 `cache/helper.go` 封装的 Cache-Aside 模式：
- **读取**：先查缓存 → miss 则查 DB → 回写缓存（TTL 带随机抖动防雪崩）
- **写入**：写 DB → 异步删除缓存（`InvalidateWithNull` 同时清除空值标记）
- **空值防穿透**：DB 查无数据时缓存空值标记 `{key}:null`，避免缓存穿透

### 追加写入模式
消息列表缓存（`message:list:*`、`message:group_list:*`）采用追加写入：
- Kafka 消费者处理新消息后，读取已有缓存 → 追加新消息 → 回写整个列表

### 异步缓存操作
大多数缓存写入/删除通过 `cache.SubmitTask()` 提交到异步工作池执行，避免阻塞主业务流程。

### TTL 策略
| 策略 | 说明 | 适用场景 |
|------|------|----------|
| `RandomizedTTL` | 基础 TTL ± 10% 随机抖动 | 用户信息、群组信息（防雪崩） |
| 固定 1 分钟 | `constants.REDIS_TIMEOUT` | 会话缓存、消息列表 |
| 固定 5 分钟 | 硬编码 | 群成员 ID 列表（Kafka 用） |
| 固定 168 小时 | `REFRESH_TOKEN_EXPIRY_HOURS` | Refresh Token ID |
| 窗口过期 | INCR + EXPIRE | 限流计数器 |
| 无 TTL | 持久 | 好友关系 Set（通过 SADD/SREM 维护） |
