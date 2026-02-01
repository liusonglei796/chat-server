# Phase 1: WebSocket 连接管理优化方案

## 优化目标
解决 WebSocket 连接稳定性问题，防止僵尸连接和资源耗尽

## 当前问题

### 1. 无心跳机制
**位置**: `internal/service/chat/ws_gateway.go:66-84`
**问题**: `Read()` 方法循环读取消息，如果客户端静默断开（如断电、断网），服务端无法感知
**风险**: 长期运行后积累大量僵尸连接，内存泄漏

### 2. 无连接数限制
**位置**: `internal/service/chat/channel_broker.go:114`
**问题**: `s.Clients.Store(client.Uuid, client)` 直接注册，没有上限检查
**风险**: 可能被恶意攻击导致资源耗尽

### 3. 无消息确认机制
**位置**: `internal/service/chat/channel_broker.go:388-393`
**问题**: 消息推送到 `SendBack` 通道即认为成功，没有确认机制
**风险**: 消息丢失无法感知

---

## 优化方案

### 1. 心跳机制设计

**实现文件**: `internal/service/chat/ws_connection.go` (新建)

**核心功能**:
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

### 2. 连接数限制设计

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

**拒绝连接响应**:
```json
{
  "type": "error",
  "code": "CONNECTION_LIMIT_REACHED",
  "message": "服务器连接数已满，请稍后重试"
}
```

### 3. 消息确认机制设计

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

**确认流程**:
1. 发送消息时生成唯一 MessageID
2. 客户端收到消息后发送 ACK
3. 服务端收到 ACK 后更新消息状态
4. 超时未收到 ACK 则重发（最多3次）

**数据库状态更新**:
```go
// 消息状态枚举扩展
const (
    Unsent  = 0  // 未发送
    Sent    = 1  // 已发送
    Acked   = 2  // 已确认
    Failed  = 3  // 发送失败
)
```

---

## 实施步骤

### Step 1: 创建连接管理器 (30分钟)
- 创建 `internal/service/chat/ws_connection.go`
- 实现 `ConnectionManager` 结构体
- 实现心跳、连接数统计、消息确认功能

### Step 2: 修改配置系统 (20分钟)
- 修改 `internal/config/config.go`，添加 WebSocketConfig
- 更新 `configs/config.toml` 模板
- 添加配置验证

### Step 3: 集成到现有代码 (40分钟)
- 修改 `ws_gateway.go:NewClientInit()`:
  - 连接前检查连接数限制
  - 连接成功后启动心跳检测
  - 断开时停止心跳

- 修改 `channel_broker.go`:
  - 发送消息时记录 MessageID
  - 处理客户端发来的 ACK 消息

### Step 4: 添加日志和监控 (20分钟)
- 连接建立/断开日志
- 心跳超时日志
- 消息确认失败日志
- 当前连接数统计接口（供监控使用）

---

## 测试方案

### 1. 心跳测试
```bash
# 使用 wscat 连接后静默
wscat -c ws://localhost:8000/ws
# 等待 90 秒，应自动断开
```

### 2. 连接数限制测试
```bash
# 使用脚本创建大量连接
for i in {1..10001}; do
  wscat -c ws://localhost:8000/ws &
done
# 第10001个应被拒绝
```

### 3. 消息确认测试
```javascript
// 客户端代码
ws.send(JSON.stringify({
  type: "text",
  content: "Hello",
  require_ack: true,
  message_id: "MSG20240101xxxx"
}));

// 收到消息后发送ACK
ws.send(JSON.stringify({
  type: "ack",
  message_id: "MSG20240101xxxx"
}));
```

---

## 风险评估

| 风险 | 可能性 | 影响 | 缓解措施 |
|------|--------|------|----------|
| 心跳增加CPU负载 | 低 | 中 | 30秒间隔足够宽松，测试验证 |
| 消息确认降低吞吐量 | 中 | 中 | 仅重要消息启用ACK，普通消息不启用 |
| 配置变更不兼容 | 低 | 高 | 提供默认值，旧配置无此字段也能运行 |
| 客户端不兼容新协议 | 中 | 中 | ACK是可选的，旧客户端不发送也能工作 |

---

## 回滚方案

如果出现问题，快速回滚步骤：
1. 恢复 `ws_gateway.go` 和 `channel_broker.go`
2. 删除 `ws_connection.go`
3. 恢复 `config.go`（移除 WebSocketConfig）
4. 重启服务

---

## 预期效果

- ✅ 僵尸连接 90 秒内被清理
- ✅ 连接数可控，防止资源耗尽
- ✅ 重要消息可达率提升到 99.9%
- ✅ 可监控当前在线用户数

---

## 下一步计划

Phase 1 完成后，继续：
- **Phase 2**: 数据库事务管理
- **Phase 3**: 配置管理强化
