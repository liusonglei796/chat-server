# 🎯 RateLimitKeyFunc 一页纸讲解

## 你的问题 + 答案

### Q：`RateLimitKeyFunc` 是啥？

**A：** 一个函数类型（函数的规格/蓝图）

```go
type RateLimitKeyFunc func(*gin.Context) string
                       ↑              ↑
                       输入          输出
                    请求信息 → 限流标识符
```

---

### Q：怎样被调用的？

**A：** 自动调用，过程如下

```
HTTP 请求
  ↓
Middleware 执行
  ↓
调用 KeyFunc(c)  ← 自动调用
  ↓
获取返回值（如 "192.168.1.1"）
  ↓
用这个值作为 Redis Key 检查计数
  ↓
超限？拒绝 ❌ : 继续 ✅
```

---

### Q：为啥这三个函数？

**A：** 不同接口需要不同的限流维度

| 函数 | 提取内容 | 用途 |
|------|--------|------|
| ByClientIP | IP 地址 | 防 IP 攻击 |
| ByFormPhone | 手机号 | 防短信轰炸 |
| ByJSONPhone | 手机号（多方式） | 支持多种请求 |

---

## 三个函数详解

### ByClientIP

```go
func ByClientIP(c *gin.Context) string {
	return c.ClientIP()
}

// 用途：防止单个 IP 的频繁请求
// 返回值：192.168.1.1 (IP 地址)
// Redis Key：rate_limit:key:192.168.1.1
```

### ByFormPhone

```go
func ByFormPhone(c *gin.Context) string {
	return c.Query("telephone")
}

// 用途：防止单个手机号的频繁请求
// 返回值：13800138000 (手机号)
// Redis Key：rate_limit:key:13800138000
// 从哪来：URL 查询参数 (?telephone=xxx)
```

### ByJSONPhone

```go
func ByJSONPhone(c *gin.Context) string {
	phone := c.PostForm("telephone")
	if phone == "" {
		phone = c.Query("telephone")
	}
	return phone
}

// 用途：防止单个手机号的频繁请求（多方式）
// 返回值：13800138000 (手机号)
// Redis Key：rate_limit:key:13800138000
// 从哪来：POST 表单数据，或 URL 查询参数
```

---

## 实际例子

### 登录接口（按 IP 限流）

```go
router.POST("/api/login",
    limiter.WithKeyFunc(ByClientIP),  // ← 使用这个
    handleLogin,
)

// 请求
POST /api/login (来自 192.168.1.1)
  ↓
KeyFunc 返回："192.168.1.1"
  ↓
Redis Key："rate_limit:key:192.168.1.1"
  ↓
如果这个 IP 请求超过 5 次/分钟 → 拒绝
```

### 发送验证码（按手机号限流）

```go
router.POST("/api/send-code",
    limiter.WithKeyFunc(ByFormPhone),  // ← 使用这个
    handleSendCode,
)

// 请求
GET /api/send-code?telephone=13800138000
  ↓
KeyFunc 返回："13800138000"
  ↓
Redis Key："rate_limit:key:13800138000"
  ↓
如果这个号码请求超过 3 次/分钟 → 拒绝
```

---

## 核心原理

### 为什么要用函数？

❌ **错误方式**（用字符串）
```go
SetKey("telephone")  // 不行！每个请求的电话号都不同
```

✅ **正确方式**（用函数）
```go
SetKeyFunc(ByFormPhone)  // 好！每次都动态获取

// 第一个请求：ByFormPhone(c) → "13800138000"
// 第二个请求：ByFormPhone(c) → "13800138001"
```

---

## Redis 中的数据

```bash
# 如果使用 ByClientIP
rate_limit:key:192.168.1.1  = 5
rate_limit:key:192.168.1.2  = 2

# 如果使用 ByFormPhone
rate_limit:key:13800138000  = 2
rate_limit:key:13800138001  = 1

# 如果组合使用
rate_limit:key:13800138000:192.168.1.1  = 2
```

---

## 自定义 KeyFunc

### 按用户 ID

```go
func ByUserID(c *gin.Context) string {
    userID, _ := c.Get("user_id")
    return userID.(string)
}

limiter.WithKeyFunc(ByUserID)
```

### 组合多个维度

```go
func ByPhoneAndIP(c *gin.Context) string {
    phone := c.Query("phone")
    ip := c.ClientIP()
    return fmt.Sprintf("%s:%s", phone, ip)
}

limiter.WithKeyFunc(ByPhoneAndIP)
```

---

## 选择哪个 KeyFunc？

| 防护场景 | 选择函数 |
|---------|--------|
| 防 DDoS 攻击 | ByClientIP |
| 防短信轰炸 | ByFormPhone |
| 防 API 滥用 | ByUserID（自定义） |
| 多维度防护 | 组合函数（自定义） |

---

## 执行流程图

```
请求来临
  ↓
Middleware 执行
  ↓
调用 KeyFunc(c)
  ├─ ByClientIP   → c.ClientIP()
  ├─ ByFormPhone  → c.Query("telephone")
  └─ ByJSONPhone  → c.PostForm() or c.Query()
  ↓
获得 Key（如 "192.168.1.1"）
  ↓
Redis GET key
  ├─ 值 > limit     → 返回 429（拒绝）
  └─ 值 <= limit    → INCR 后继续
```

---

## 关键点

✅ **KeyFunc 是函数类型** - 用来定义提取 Key 的规则

✅ **自动调用** - 请求来临时中间件自动调用

✅ **三个现成的** - 覆盖最常见的场景

✅ **可自定义** - 写自己的 KeyFunc 也很简单

✅ **返回值作为 Key** - 用来在 Redis 中追踪限流

---

## 最后总结

```
RateLimitKeyFunc
  ↓
从请求中提取标识符的函数
  ↓
三种常见方式：
  1. 按 IP 提取       (ByClientIP)
  2. 按手机号提取     (ByFormPhone)
  3. 按手机号提取     (ByJSONPhone，多方式)
     （带回退方式）
  ↓
用标识符作为 Redis Key 检查计数
  ↓
超限则拒绝，正常则继续
```

---

现在你完全理解 `RateLimitKeyFunc` 是什么、怎样被调用、为什么有这三个函数了！

想深入了解？查看：
- `RATELIMIT_KEYFUNC_EXPLAINED.md`（详尽）
- `KEYFUNC_QUICK_REFERENCE.md`（快速查表）
- `KEYFUNC_PRACTICE.md`（实践指南）
