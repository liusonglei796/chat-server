# SMS 验证码登录系统 - 完整架构说明

## 📊 系统架构

```
┌──────────────────────────────────────────────────────────────────────┐
│                          客户端                                       │
│                 (浏览器 / 移动应用)                                    │
└──────────────────────────────────────────────────────────────────────┘
                              │ HTTPS
                              ▼
┌──────────────────────────────────────────────────────────────────────┐
│                    Nginx SSL 终止代理                                 │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ • SSL/TLS 加解密（443 → 8080）                                │  │
│  │ • 速率限制（IP/手机号维度）                                    │  │
│  │ • 反向代理转发                                                 │  │
│  │ • 错误处理与降级                                               │  │
│  └────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────┘
                              │ HTTP
                              ▼
┌──────────────────────────────────────────────────────────────────────┐
│                 应用服务（Go HTTP Server）                            │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ /api/send-code        ← 发送验证码                            │  │
│  │ /api/verify-code      ← 校验验证码                            │  │
│  │ /api/login            ← 完整登录流程                          │  │
│  │ /health               ← 健康检查                              │  │
│  │ /metrics              ← 限流指标                              │  │
│  └────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────┘
         │                              │                    │
         ▼                              ▼                    ▼
    [Redis存储]                  [阿里云SMS]          [日志输出]
    验证码缓存                    短信网关              业务日志
    计数限流
```

---

## 🔐 三层核心设计解析

### 1️⃣ **阿里云短信服务 + 预占与回滚机制**

#### **问题场景**
- 多个用户同时请求发送验证码
- 阿里云有日配额限制（如 1000 条/天）
- 不能让用户请求到一半才发现超配额

#### **解决方案：预占机制**

```
时间轴：
T1: 用户A → 额度检查 → DECR(quota) = 999 → 发送 SMS → ✅ 成功，额度已扣减
                              ↓
T2: 用户B → 额度检查 → DECR(quota) = 998 → 发送 SMS → ❌ 失败，SMS API超时
                              ↓
               立即回滚 → INCR(quota) = 999（恢复一个配额）
```

**Redis 操作序列：**
```go
// 1. 预占额度
quotaKey := "sms:quota:2024-01-15"
quotaRemain := redis.Decr(quotaKey)  // 原子操作

if quotaRemain < 0 {
    redis.Incr(quotaKey)  // 额度不足，立即恢复
    return Error("配额已用尽")
}

// 2. 调用阿里云 API
resp, err := aliyun.SendSMS(phone, code)

// 3. 失败回滚
if err != nil {
    redis.Incr(quotaKey)  // 恢复配额
    return err
}

// 4. 成功，额度保持扣减状态（持久化）
```

#### **频率限制维度**
- **用户维度**: 同一手机号 1 分钟最多 3 次
- **IP 维度**: 同一 IP 1 分钟最多 10 次
- **日配额**: 同一用户每天最多 10 条

**对应 Redis Key：**
```
sms:limit:user:{phone}        → INCR，60s TTL（用户频率）
sms:limit:ip:{ip}             → INCR，60s TTL（IP 频率）
sms:quota:{date}:{phone}      → DECR，24h TTL（日配额）
sms:code:{phone}              → JSON，5min TTL（验证码存储）
```

---

### 2️⃣ **Redis 固定窗口限流中间件**

#### **固定窗口 vs 滑动窗口**

```
固定窗口（简单高效）：
|----[0-10s]------[10-20s]------[20-30s]----|
     5个请求       8个请求       3个请求
     （未超限）     （未超限）     （未超限）

缺点：窗口边界可能突发
T=9.5s 来 6 个 + T=10.5s 来 6 个 = 12 个请求在 1s 内！


滑动窗口（精准但开销大）：
时间 ──→
|  1  |  2  |  3  |  4  |  5  |  6  |  7  |  8  |  9  | 10 |
  ✓   ✓    ✓   ✗   ✗   ✓    ✓   ✓   ✗    ✗

在 [4, 10] 时间段内移动窗口，只计算最近 10s 的请求
```

#### **本实现使用固定窗口**

**配置：**
```go
RateLimitConfig{
    WindowSize:      1 * time.Second,  // 窗口大小
    MaxRequests:     10,               // 最大请求数
    DegradeFallback: true,             // Redis 故障时放通
}
```

**工作流程：**
```
请求来临
  ↓
key := "rate_limit:ip:192.168.1.1"
  ↓
count := INCR(key)  // 原子增 1
  ↓
EXPIRE(key, 1s)     // 设置过期
  ↓
if count <= 10 {
    ✅ 通过
} else {
    ❌ 被限流（返回 429 Too Many Requests）
}
```

#### **降级机制**

```
Redis 正常:
请求 → Redis INCR → 检查限制 → 放通/拒绝 ✅

Redis 故障:
请求 → Redis 连接超时 → 降级打开 → 直接放通 ✅（或返回 503）
```

**指标收集：**
```json
{
    "TotalRequests": 1523,    // 总请求数
    "RejectedReqs": 45,       // 被限流请求
    "DegradedReqs": 12        // 降级模式下的请求
}
```

---

### 3️⃣ **Nginx SSL 终止代理**

#### **为什么需要 SSL 终止？**

**方案对比：**

```
❌ 应用层 TLS：
请求 → [Nginx] → [App 加密解密] → CPU 占用高
                    ↓
            业务处理受阻

✅ SSL 终止：
请求 → [Nginx 加解密] → [App HTTP] → 业务逻辑优先
                   CPU 优化分离
```

#### **Nginx 配置要点**

**1. HTTPS 入站（443）→ HTTP 转发（8080）**
```nginx
server {
    listen 443 ssl http2;
    ssl_certificate /etc/nginx/certs/cert.pem;
    ssl_certificate_key /etc/nginx/certs/key.pem;
    
    # 转发到内部应用
    proxy_pass http://app_backend;  # 不需要 HTTPS
}
```

**2. 差异化限流**
```nginx
# 严格限制验证码接口（3 req/min）
location /api/send-code {
    limit_req zone=phone_limit burst=1 nodelay;
    limit_req zone=api_limit burst=20 nodelay;
    proxy_pass http://app_backend;
}

# 普通 API 接口（10 req/s）
location /api/ {
    limit_req zone=api_limit burst=30 nodelay;
    proxy_pass http://app_backend;
}
```

**3. 客户端信息传递**
```nginx
# 应用层能获取到真实客户端 IP
proxy_set_header X-Real-IP $remote_addr;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto https;
```

**4. 性能优化**
```nginx
# HTTP/2 复用连接
listen 443 ssl http2;

# 连接池
upstream app_backend {
    keepalive 32;
}

# 缓冲配置
proxy_buffering on;
proxy_buffer_size 4k;
proxy_buffers 8 4k;
```

**5. 错误处理与重试**
```nginx
proxy_next_upstream error timeout http_502 http_503;
proxy_next_upstream_tries 2;
```

---

## 🚀 部署与运行

### **快速启动**
```bash
# 1. 生成自签名证书
bash setup.sh

# 2. 配置环境变量
export ALIYUN_ACCESS_KEY="xxxxxxxx"
export ALIYUN_ACCESS_SECRET="xxxxxxxx"
export SMS_SIGN_NAME="阿里云短信"
export SMS_TEMPLATE_ID="SMS_123456789"

# 3. 启动容器
docker compose up --build

# 4. 运行测试
bash test.sh
```

---

## 📈 生产环保景观最佳实践

### **1. 监控指标**
```
• Redis 连接池使用率
• 验证码发送成功率
• 限流拒绝率
• Nginx SSL 握手时间
• 平均响应时间
```

### **2. 告警阈值**
```
• 验证码失败率 > 5% → 告警
• 限流拒绝率 > 10% → 告警
• Redis 连接失败 → 立即告警
• SSL 握手时间 > 100ms → 告警
```

### **3. 容量规划**
```
QPS 1000:
- Nginx: 2 实例（100 连接/s 处理能力）
- App: 4 实例（250 QPS/实例）
- Redis: 1 主 2 从（足够处理计数）

Redis 内存估算:
- 100K 活跃用户 × 验证码 (200B) = 20MB
- 限流计数 × 100K IP = 10MB
- 其他缓存 = 50MB
总计：80MB （留余量到 300MB）
```

### **4. 灾备方案**
```
Redis 故障 → 降级放通 → 等待恢复（可能会有虐用，但服务不中断）
Aliyun API 故障 → 重试 3 次 → 返回 503（提示用户稍后重试）
Nginx 故障 → 负载均衡自动切换（多 Nginx 实例）
```

---

## 🔗 相关概念

| 概念 | 说明 |
|------|------|
| **预占** | 操作前先扣减资源，失败时回滚恢复 |
| **固定窗口** | 按时间段计数，简单高效但边界可能突发 |
| **滑动窗口** | 精确限流但开销大，用于严格场景 |
| **降级** | 依赖服务故障时，主动切换备用方案 |
| **SSL 终止** | 代理层处理加解密，应用层只处理业务 |
| **HTTP/2** | 多路复用，提升连接利用率 |

---

## 📝 常见问题

### Q: 为什么验证码存储在 Redis，不在 MySQL？
A: 验证码是临时数据（5 分钟有效），Redis 基于内存速度快，且支持原子操作和 TTL。

### Q: 预占失败时为什么要回滚？
A: 防止余额不足导致 API 调用失败，造成用户重试 → 浪费配额 → 级联故障。

### Q: Nginx 限流和应用限流有什么区别？
A: Nginx 是全局限流（保护整个系统），应用限流是业务级限流（保护特定接口）。

### Q: 如何处理 Nginx 后面有多个应用实例？
A: 使用 `upstream` 指令配置负载均衡，Nginx 自动分发请求。

---
