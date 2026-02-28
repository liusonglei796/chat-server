# 聊天消息流转全流程文档

本文档详细描述一条消息从用户 A 发出到用户 B 收到的完整链路，涉及的所有结构体、方法和数据变换。

---

## 一、架构总览

```
┌─────────────┐         ┌──────────────┐         ┌─────────────┐
│  用户 A 浏览器 │◄──WS──►│   Go 服务器    │◄──WS──►│ 用户 B 浏览器  │
└─────────────┘         │              │         └─────────────┘
                        │  ┌────────┐  │
                        │  │ Kafka  │  │
                        │  │ Cluster│  │
                        │  └────────┘  │
                        └──────────────┘
```

用户只通过 WebSocket 与 Go 服务器通信，**不感知 Kafka 的存在**。

---

## 二、核心文件清单

| 文件 | 职责 |
|------|------|
| `internal/handler/ws_handler.go` | HTTP → WebSocket 升级入口 |
| `internal/service/chat/ws_gateway.go` | WebSocket 连接管理、消息读写 |
| `internal/service/chat/server.go` | 聚合结构、MsgConsumer |
| `internal/service/chat/kafka_client.go` | Kafka 生产者/消费者底层封装 |
| `internal/service/chat/kafka_broker.go` | Kafka 消息消费、业务处理、消息路由（MsgConsumer 实现） |
| `internal/dto/request/message/chat_message_request.go` | 消息请求 DTO |
| `internal/dto/respond/message/get_message_list_respond.go` | 消息响应 DTO |
| `internal/dto/respond/message/av_message_respond.go` | 音视频消息响应 DTO |
| `internal/model/message.go` | 数据库 Message 模型 |

---

## 三、核心结构体一览

### 3.1 连接层

```go
// ws_gateway.go — WebSocket 连接实体（1个在线用户 = 1个实例）
type UserConn struct {
    Conn        *websocket.Conn   // 底层 WebSocket 连接
    Uuid        string            // 用户ID（来自 JWT 认证，可信）
    SendBack    chan *MessageBack  // 待推送给前端的消息队列
    broker      *MsgConsumer     // 注入的消息消费者
    cleanupOnce sync.Once         // 确保资源只释放一次
}

// ws_gateway.go — 推送给前端的消息载体（1条消息 = 1个实例）
type MessageBack struct {
    Message []byte  // 序列化后的 JSON 响应体
    Uuid    string  // 消息ID，用于 Write 协程更新消息状态为"已发送"
}
```

### 3.2 请求/响应 DTO

```go
// dto/request/message/chat_message_request.go — 消息请求
type ChatMessageRequest struct {
    SessionId  string `json:"session_id"`                    // 会话ID
    Type       int8   `json:"type" binding:"required"`       // 消息类型（0文本/1语音/2文件/3音视频）
    Content    string `json:"content"`                       // 文本内容
    Url        string `json:"url"`                           // 文件URL
    SendId     string `json:"send_id" binding:"required"`    // 发送者ID（⚠️ 服务端注入，非客户端传入）
    SendName   string `json:"send_name"`                     // 发送者昵称
    SendAvatar string `json:"send_avatar"`                   // 发送者头像
    ReceiveId  string `json:"receive_id" binding:"required"` // 接收者ID（U开头=用户，G开头=群组）
    FileSize   string `json:"file_size"`                     // 文件大小
    FileType   string `json:"file_type"`                     // 文件MIME类型
    FileName   string `json:"file_name"`                     // 文件名
    AVdata     string `json:"av_data"`                       // 音视频信令数据（JSON字符串）
}

// dto/respond/message/get_message_list_respond.go — 普通消息响应（推送给前端）
type GetMessageListRespond struct {
    SendId     string `json:"send_id"`
    SendName   string `json:"send_name"`
    SendAvatar string `json:"send_avatar"`
    ReceiveId  string `json:"receive_id"`
    Type       int8   `json:"type"`
    Content    string `json:"content"`
    Url        string `json:"url"`
    FileType   string `json:"file_type"`
    FileName   string `json:"file_name"`
    FileSize   string `json:"file_size"`
    CreatedAt  string `json:"created_at"`
}

// dto/respond/message/av_message_respond.go — 音视频消息响应（比普通响应多 AVdata 字段）
type AVMessageRespond struct {
    // ...与 GetMessageListRespond 相同的字段...
    AVdata string `json:"av_data"` // 音视频信令数据
}
```

### 3.3 数据库模型

```go
// model/message.go — 持久化到 MySQL 的消息实体
type Message struct {
    gorm.Model                               // ID, CreatedAt, UpdatedAt, DeletedAt
    Uuid       string `gorm:"column:uuid"`   // "M" + 雪花ID
    SessionId  string                        // 所属会话
    Type       int8                          // 消息类型
    Content    string                        // 文本内容
    Url        string                        // 文件URL
    SendId     string                        // 发送者UUID
    SendName   string                        // 发送者昵称（冗余）
    SendAvatar string                        // 发送者头像（冗余）
    ReceiveId  string                        // 接收者UUID
    FileType   string                        // 文件MIME
    FileName   string                        // 文件名
    FileSize   string                        // 文件大小
    Status     int8                          // 0=未发送, 1=已发送
    SendAt     sql.NullTime                  // 实际发送时间
    AVdata     string                        // 音视频信令JSON
}
```

### 3.4 Broker 层

```go
// kafka_broker.go — Kafka 消息消费者
type MsgConsumer struct {
    Clients         sync.Map                    // 在线用户表 map[userUUID]*UserConn
    Login           chan *UserConn               // 登录事件通道
    Logout          chan *UserConn               // 登出事件通道
    kafkaClient     *KafkaClient                // Kafka 读写客户端
    messageRepo     mysql.MessageRepository     // 消息持久化
    friendshipRepo  mysql.FriendshipRepository  // 好友关系校验
    groupMemberRepo mysql.GroupMemberRepository  // 群成员校验
    sessionRepo     mysql.SessionRepository     // 更新会话最后消息
    cacheService    myredis.AsyncCacheService   // Redis 缓存
}

// 发布消息到 Kafka
func (k *MsgConsumer) Publish(ctx context.Context, msg []byte) error {
    return k.kafkaClient.SendMessage(ctx, []byte("0"), msg)
}

// 获取消息仓库
func (k *MsgConsumer) GetMessageRepo() mysql.MessageRepository {
    return k.messageRepo
}
```

### 3.5 Kafka 客户端
type KafkaClient struct {
    Producer  *kafka.Writer  // 写入 Kafka
    Consumer  *kafka.Reader  // 从 Kafka 读取
}
```

### 3.5 枚举常量

```go
// pkg/enum/message/message_type — 消息类型
const (
    Text             = 0   // 文本
    Voice            = 1   // 语音
    File             = 2   // 文件
    AudioOrVideo     = 3   // 音视频通话信令
    KickNotification = 99  // 踢人通知
)

// pkg/enum/message/message_status — 消息状态
const (
    Unsent = 0  // 未发送（入库时的初始状态）
    Sent   = 1  // 已发送（WebSocket 推送成功后更新）
)
```

---

## 四、完整消息流转（以用户 A 发文本消息给用户 B 为例）

### 阶段 1：建立连接

```
用户 A 浏览器                       Go 服务器
    │                                  │
    │─── GET /ws/login ───────────────►│  ① HTTP 请求（携带 JWT）
    │    (携带 Authorization Header)    │
    │                                  │  ② WsHandler.WsLoginHandler()
    │                                  │     从 c.Get("user_id") 获取可信的 userId
    │                                  │
    │                                  │  ③ NewClientInit(c, clientId, broker)
    │                                  │     upgrader.Upgrade() 升级为 WebSocket
    │                                  │     创建 UserConn{Uuid: "U12345", ...}
    │                                  │     broker.RegisterClient(client)
    │                                  │       → Login channel → Clients.Store()
    │                                  │     go client.Read()   // 启动读协程
    │                                  │     go client.Write()  // 启动写协程
    │◄── WebSocket 连接建立 ──────────►│
    │◄── "欢迎来到kama聊天服务器" ──────│
```

**涉及方法（读写标注）：**

| 方法 | 位置 | 读取自 | 写入到 |
|------|------|--------|--------|
| `WsHandler.WsLoginHandler()` | `ws_handler.go:32` | Gin Context (`c.Get("user_id")`) | — |
| `NewClientInit()` | `ws_gateway.go:206` | HTTP Request | `UserConn` 实例 |
| `upgrader.Upgrade()` | `ws_gateway.go:207` | HTTP 连接 | WebSocket 连接 (`*websocket.Conn`) |
| `broker.RegisterClient()` | `kafka_broker.go:224` | `*UserConn` 参数 | `Login` channel → `Clients` sync.Map |

---

### 阶段 2：发送消息（上行：浏览器 → Kafka）

```
用户 A 浏览器                       Go 服务器                        Kafka
    │                                  │                              │
    │─── WebSocket TextMessage ───────►│                              │
    │    {                             │                              │
    │      "type": 0,                  │  ④ UserConn.Read()           │
    │      "content": "你好",          │     ReadMessage() 读取 JSON   │
    │      "receive_id": "U67890",     │                              │
    │      "send_name": "Alice",       │  ⑤ injectSenderId()         │
    │      "send_avatar": "/static/…", │     注入 send_id: "U12345"   │
    │      "session_id": "S00001"      │     （服务端从 JWT 获取）      │
    │    }                             │                              │
    │    ⚠️ 注意：不含 send_id         │  ⑥ broker.Publish(msg)       │
    │                                  │     → KafkaClient.SendMessage()
    │                                  │─── kafka.WriteMessages ──────►│
    │                                  │                              │ 消息入队
```

**涉及方法（读写标注）：**

| 方法 | 位置 | 读取自 | 写入到 |
|------|------|--------|--------|
| `Conn.ReadMessage()` | `ws_gateway.go:87` | WebSocket 连接 | `jsonMessage []byte`（原始 JSON，无 send_id） |
| `injectSenderId()` | `ws_gateway.go:143` | `jsonMessage` + `UserConn.Uuid` | `securedMessage []byte`（注入 send_id 后的 JSON） |
| `broker.Publish()` | `kafka_broker.go:88` | `securedMessage []byte` | Kafka Topic `chat_message` |
| `KafkaClient.SendMessage()` | `kafka_client.go:74` | `key, value []byte` | Kafka Producer (`kafka.Writer`) |

---

### 阶段 3：消费消息（下行：Kafka → 业务处理）

```
Kafka                           Go 服务器
  │                                │
  │─── kafkaMessage ──────────────►│  ⑦ Consumer.ReadMessage()
  │    Value: 完整 JSON            │     阻塞等待 Kafka 消息
  │                                │
  │                                │  ⑧ json.Unmarshal → ChatMessageRequest
  │                                │     反序列化为请求 DTO
  │                                │
  │                                │  ⑨ switch req.Type
  │                                │     case Text  → handleTextMessage()
  │                                │     case File  → handleFileMessage()
  │                                │     case AV    → handleAVMessage()
```

**涉及方法（读写标注）：**

| 方法 | 位置 | 读取自 | 写入到 |
|------|------|--------|--------|
| `Consumer.ReadMessage()` | `kafka_broker.go:127` | Kafka Topic `chat_message` | `kafkaMessage.Value []byte` |
| `json.Unmarshal()` | `kafka_broker.go:144` | `kafkaMessage.Value []byte` | `ChatMessageRequest` 结构体 |

---

### 阶段 4：处理文本消息（以 handleTextMessage 为例）

```
handleTextMessage(req ChatMessageRequest)
    │
    │  ⑩ checkSendPermission(req.SendId, req.ReceiveId)
    │     ├─ ReceiveId 以 "U" 开头 → friendshipRepo.IsFriend() 校验好友关系
    │     └─ ReceiveId 以 "G" 开头 → groupMemberRepo.FindByGroupAndUser() 校验群成员
    │     （校验失败 → sendPermissionError() 推送错误给发送者，return）
    │
    │  ⑪ 构建 model.Message
    │     Uuid:   "M" + snowflake.GenerateIDString()
    │     Status: message_status.Unsent (= 0)
    │     其他字段从 req 映射
    │
    │  ⑫ messageRepo.Create(&message)
    │     持久化到 MySQL message 表
    │
    │  ⑬ go sessionRepo.UpdateLastMessage(...)
    │     异步更新会话的最后消息摘要（用于会话列表展示）
    │
    │  ⑭ 路由分发
    │     ├─ ReceiveId[0] == 'U' → dispatchToUser()   // 私聊
    │     └─ ReceiveId[0] == 'G' → dispatchToGroup()  // 群聊
```

**涉及方法（读写标注）：**

| 方法 | 位置 | 读取自 | 写入到 |
|------|------|--------|--------|
| `checkSendPermission()` | `kafka_broker.go:273` | `friendshipRepo` / `groupMemberRepo`（读 MySQL） | —（仅校验，返回 error） |
| `sendPermissionError()` | `kafka_broker.go:304` | 错误原因字符串 | 发送者的 `UserConn.SendBack` channel |
| 构建 `model.Message` | `kafka_broker.go:346-361` | `ChatMessageRequest` 各字段 | `model.Message` 结构体 |
| `messageRepo.Create()` | `kafka_broker.go:367` | `model.Message` 结构体 | MySQL `message` 表 |
| `sessionRepo.UpdateLastMessage()` | `kafka_broker.go:375` | `model.Message` 字段 | MySQL `session` 表（异步） |

---

### 阶段 5：分发到 channel（私聊 dispatchToUser）

```
dispatchToUser(message model.Message, originalAvatar string)
    │
    │  ⑮ 构建 GetMessageListRespond（响应 DTO）
    │     从 model.Message 映射字段
    │     CreatedAt 格式化为 "2006-01-02 15:04:05"
    │
    │  ⑯ json.Marshal → jsonMessage []byte
    │
    │  ⑰ 包装为 MessageBack{Message: jsonMessage, Uuid: message.Uuid}
    │
    │  ⑱ 推送给接收者 B
    │     Clients.Load("U67890") → receiveClient
    │     trySendBack(receiveClient, messageBack)
    │       → receiveClient.SendBack <- messageBack  （非阻塞写入）
    │
    │  ⑲ 回显给发送者 A
    │     Clients.Load("U12345") → sendClient
    │     trySendBack(sendClient, messageBack)
    │       → sendClient.SendBack <- messageBack
    │
    │  ⑳ 异步更新 Redis 缓存
    │     cacheService.SubmitTask(func() { ... })
```

**涉及方法（读写标注）：**

| 方法 | 位置 | 读取自 | 写入到 |
|------|------|--------|--------|
| 构建 `GetMessageListRespond` | `kafka_broker.go:532-544` | `model.Message` 结构体 | `GetMessageListRespond` 响应 DTO |
| `json.Marshal()` | `kafka_broker.go:546` | `GetMessageListRespond` 结构体 | `jsonMessage []byte` |
| 构建 `MessageBack` | `kafka_broker.go:553-556` | `jsonMessage` + `message.Uuid` | `MessageBack` 结构体 |
| `Clients.Load()` | `kafka_broker.go:560,565` | `Clients` sync.Map（key=用户UUID） | `*UserConn`（接收者/发送者连接） |
| `trySendBack()` | `kafka_broker.go:321` | `*MessageBack` 参数 | `UserConn.SendBack` channel（非阻塞写入） |
| `cacheService.Set()` | `kafka_broker.go:585` | `GetMessageListRespond` 追加到已有列表 | Redis Key `message:list:{id1}_{id2}`（异步） |

---

### 💡 为什么阶段 5 不直接写 WebSocket，还需要阶段 6？

阶段 5 的 `dispatchToUser()` / `dispatchToGroup()` 中，`trySendBack()` 只是把消息写入了 Go 的内存 channel（`UserConn.SendBack`），**消息此时仍在 Go 进程内存中，尚未离开服务器，用户浏览器完全不知道**。

必须由阶段 6 的 Write goroutine 从 channel 取出消息，调用 `Conn.WriteMessage()` 才能真正通过 TCP 连接推送到浏览器。

**这么设计的核心原因是 `gorilla/websocket` 的 `Conn` 不支持并发写**：

```
                    ┌─────────────────────────┐
                    │   UserConn.SendBack     │
                    │     (内存 channel)       │
                    │  ┌───┬───┬───┬───┐      │
  Kafka 消费者 ────►│  │msg│msg│msg│...│      │
  Ping 心跳定时器 ──┤  └───┴───┴───┴───┘      │
  权限错误推送 ─────┤         │                │
                    └─────────│────────────────┘
                              ▼
                    ┌─────────────────────────┐
                    │  Write goroutine (独占)   │
                    │  Conn.WriteMessage()     │──── WebSocket ────► 用户浏览器
                    │  Conn.PingMessage()      │
                    └─────────────────────────┘
```

| 角色 | 能写 SendBack channel | 能写 WebSocket 连接 |
|------|:--------------------:|:------------------:|
| Kafka 消费者 goroutine | ✅ `trySendBack()` | ❌ 禁止 |
| Write goroutine | ❌ 只读 channel | ✅ 独占写入 |

如果多个 goroutine 同时调用 `Conn.WriteMessage()`，会导致数据错乱或 panic。通过 channel 解耦实现了经典的 **fan-in 模式**：多个生产者（Kafka 消费者、心跳定时器等）汇聚到一个 channel，由唯一的 Write goroutine 串行写出。

---

### 阶段 6：WebSocket 写出（Write 协程）

```
Go 服务器                          用户 B 浏览器
    │                                  │
    │  ㉑ UserConn.Write()              │
    │     select ← SendBack channel     │
    │     收到 messageBack              │
    │                                  │
    │  ㉒ Conn.WriteMessage(TextMessage, messageBack.Message)
    │─── WebSocket TextMessage ───────►│  用户 B 看到消息
    │    {                             │
    │      "send_id": "U12345",        │
    │      "send_name": "Alice",       │
    │      "receive_id": "U67890",     │
    │      "type": 0,                  │
    │      "content": "你好",          │
    │      "created_at": "2026-02-07…" │
    │    }                             │
    │                                  │
    │  ㉓ messageRepo.UpdateStatus(uuid, Sent)
    │     消息状态: Unsent(0) → Sent(1)│
```

**涉及方法（读写标注）：**

| 方法 | 位置 | 读取自 | 写入到 |
|------|------|--------|--------|
| `<-SendBack` | `ws_gateway.go:171` | `UserConn.SendBack` channel | `messageBack` 局部变量 |
| `Conn.WriteMessage()` | `ws_gateway.go:181` | `messageBack.Message []byte` | WebSocket 连接 → 用户浏览器 |
| `messageRepo.UpdateStatus()` | `ws_gateway.go:188` | `messageBack.Uuid` + `message_status.Sent` | MySQL `message` 表 `status` 字段 |

---

## 五、群聊流程差异（dispatchToGroup）

与私聊的区别仅在阶段 5：

```
dispatchToGroup(message, originalAvatar)
    │
    │  构建 GetMessageListRespond（同私聊）
    │
    │  getGroupMembersCached(groupId)
    │     ├─ 优先从 Redis 读取群成员ID列表（Key: "group:member_ids:{groupId}"）
    │     └─ 未命中 → groupMemberRepo.FindByGroupUuid() 查 DB → 回写缓存（TTL 5分钟）
    │
    │  遍历群成员:
    │     ├─ 非发送者 → Clients.Load(memberId) → trySendBack()  // 推送
    │     └─ 发送者自己 → trySendBack()                          // 回显
    │
    │  异步更新群聊消息缓存（Key: "message:group_list:{groupId}"）
```

**涉及方法（读写标注）：**

| 方法 | 位置 | 读取自 | 写入到 |
|------|------|--------|--------|
| `getGroupMembersCached()` | `kafka_broker.go:670` | Redis Key `group:member_ids:{groupId}` 或 MySQL `group_member` 表 | `[]model.GroupMember`；缓存 miss 时回写 Redis（TTL 5分钟） |
| `Clients.Load()` | `kafka_broker.go:637` | `Clients` sync.Map（key=成员UUID） | `*UserConn`（各成员连接） |
| `trySendBack()` | `kafka_broker.go:321` | `*MessageBack` 参数 | 各成员 `UserConn.SendBack` channel |
| `cacheService.Set()` | `kafka_broker.go:660` | `GetMessageListRespond` 追加到已有列表 | Redis Key `message:group_list:{groupId}`（异步） |

---

## 六、音视频信令流程差异（handleAVMessage）

```
handleAVMessage(req)
    │
    │  解析 req.AVdata → AVData{MessageId, Type}
    │
    │  仅关键信令入库（MessageId=="PROXY" 且 Type 为 start_call/receive_call/reject_call）
    │  其他信令（如 WebRTC offer/answer/candidate）仅转发，不持久化
    │
    │  构建 AVMessageRespond（比普通响应多 AVdata 字段）
    │  仅推送给接收者，不回显给发送者
```

**涉及方法（读写标注）：**

| 方法 | 位置 | 读取自 | 写入到 |
|------|------|--------|--------|
| `json.Unmarshal(req.AVdata)` | `kafka_broker.go:456` | `req.AVdata` JSON 字符串 | `AVData` 结构体 |
| `messageRepo.Create()` | `kafka_broker.go:483` | `model.Message` 结构体 | MySQL `message` 表（仅关键信令） |
| 构建 `AVMessageRespond` | `kafka_broker.go:492-505` | `model.Message` 结构体 | `AVMessageRespond` 响应 DTO |
| `Clients.Load()` | `kafka_broker.go:518` | `Clients` sync.Map（key=接收者UUID） | `*UserConn`（接收者连接） |
| `trySendBack()` | `kafka_broker.go:321` | `*MessageBack` 参数 | 接收者 `UserConn.SendBack` channel |

---

## 七、连接生命周期

### 建立连接
```
HTTP GET /ws/login (JWT认证)
  → WsHandler.WsLoginHandler()
  → NewClientInit()
  → upgrader.Upgrade()  → 创建 UserConn
  → broker.RegisterClient()  → Login channel → Clients.Store()
  → go Read()  // 读协程
  → go Write() // 写协程
```

### 心跳保活
```
Write 协程每 54 秒发送 Ping 帧 (pingPeriod = pongWait * 9/10)
客户端回复 Pong → PongHandler 续期读超时 (pongWait = 60秒)
超时未收到 Pong → Read 协程退出 → 触发 cleanup
```

### 断开连接（3种触发方式）
```
1. 客户端主动断开 → Read() 读到 CloseError → defer cleanup()
2. 客户端调用登出 → POST /ws/logout → ClientLogout() → cleanup()
3. 单点登录互踢 → KickClient() → 推送踢人消息 → cleanup()
```

### cleanup 资源释放顺序
```
cleanup() (sync.Once 保证只执行一次)
  ① broker.UnregisterClient(client)  → Logout channel → Clients.Delete()
  ② close(SendBack)                  → Write 协程退出
  ③ Conn.Close()                     → WebSocket 连接关闭
```

---

## 八、数据流全景图

```
┌─────────┐     ┌──────────┐     ┌─────────────┐     ┌───────┐     ┌──────────────┐
│ 客户端 A  │────►│ UserConn │────►│ injectSenderId│────►│Publish│────►│ Kafka Topic  │
│(浏览器)   │ WS  │  .Read() │JSON │  注入send_id  │JSON │  ()   │     │chat_message  │
└─────────┘     └──────────┘     └─────────────┘     └───────┘     └──────┬───────┘
                                                                          │
                                                              Consumer.ReadMessage()
                                                                          │
                                                                          ▼
┌─────────┐     ┌──────────┐     ┌────────────┐     ┌──────────────────────────────┐
│ 客户端 B  │◄───│ UserConn │◄───│ MessageBack│◄───│ handleTextMessage()           │
│(浏览器)   │ WS │  .Write()│chan │ {JSON,UUID}│     │  ① checkSendPermission()     │
└─────────┘     └──────────┘     └────────────┘     │  ② model.Message 入库 MySQL   │
                                                     │  ③ dispatchToUser/dispatchToGroup()  │
                                                     │     → GetMessageListRespond   │
                                                     │     → json.Marshal            │
                                                     │     → trySendBack(SendBack←)  │
                                                     │  ④ 异步更新 Redis 缓存         │
                                                     └──────────────────────────────┘
```
