# 15. WebSocket 基础与连接管理

> 本章从项目的真实实现出发，讲清楚 WebSocket 连接如何升级、如何管理在线用户、以及消息如何通过 `MessageBroker`（Kafka 模式）流转。

---

## 📌 学习目标

- 理解 WebSocket 的连接特性与适用场景
- 掌握 `gorilla/websocket` 将 HTTP 升级为 WebSocket
- 理解 `UserConn`（连接对象）的读写协程模型
- 理解 `MessageBroker` 抽象与依赖注入（Kafka 模式）
- 理解当前项目的路由与鉴权要求（`/ws` 需要 Bearer Access Token）

---

## 1. WebSocket 简介

WebSocket 是在单个 TCP 连接上进行全双工通信的协议，适合聊天、实时推送等低延迟场景。

---

## 2. 项目结构说明（以代码为准）

WebSocket 与聊天服务核心代码位于：

```
internal/service/chat/
├── ws_gateway.go         # WebSocket 升级 + UserConn 读写协程
├── server.go             # MessageBroker 接口 + ChatServer 组装
├── kafka_broker.go       # MsgConsumer（Kafka 模式消息代理）
└── kafka_client.go       # Kafka 客户端封装
```

路由与 Handler：

- WebSocket 入口路由：`internal/router/ws_routes.go`
- WebSocket Handler：`internal/handler/ws_handler.go`
- JWT 中间件：`internal/infrastructure/middleware/jwt_middleware.go`

---

## 3. 核心数据结构

### 3.1 MessageBack：回传给前端的消息包

```go
type MessageBack struct {
	Message []byte // 序列化后的 JSON 响应体
	Uuid    string // 消息唯一标识，用于推送成功后更新 message.status = Sent
}
```

### 3.2 UserConn：一个 WebSocket 客户端连接

```go
type UserConn struct {
	Conn        *websocket.Conn   // 底层 WebSocket 连接
	Uuid        string            // 用户 ID（来自 JWT，可信）
	SendBack    chan *MessageBack // 待推送消息队列
	broker      MessageBroker     // 注入的消息代理
	cleanupOnce sync.Once         // 确保 cleanup 只执行一次
}
```

*文件位置：`internal/service/chat/ws_gateway.go`*

字段说明：

- `Conn`：WebSocket 连接对象
- `Uuid`：用户 UUID（同时作为在线表 key）
- `SendBack`：服务端推送给该连接的通道
- `broker`：用于发布消息与在线管理的抽象接口
- `cleanupOnce`：通过 sync.Once 确保资源清理只执行一次

### 3.3 Upgrader：连接升级器

```go
var upgrader = websocket.Upgrader{
    ReadBufferSize:  2048,
    WriteBufferSize: 2048,
    CheckOrigin: func(r *http.Request) bool { return true },
}
```

*文件位置：`internal/service/chat/ws_gateway.go`*

> 注意：`CheckOrigin: return true` 允许跨域连接，生产环境建议按域名收敛。

### 3.4 常量

```go
const (
	CHANNEL_SIZE               = 100      // 通道大小
	FILE_MAX_SIZE              = 30 << 20 // 文件最大 30 MB
	REDIS_TIMEOUT              = 1        // 分钟
)
```

*文件位置：`pkg/constants/constants.go`*

---

## 4. UserConn 读写协程（ws_gateway）

UserConn 不感知底层消息代理实现，由 broker 决定消息路由方式。

### 4.1 Read：读取消息并交给 Broker

```go
func (c *UserConn) Read() {
	defer c.cleanup() // cleanup 确保资源释放

	// 设置心跳超时
	_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		_ = c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	zap.L().Info("ws read goroutine start", zap.String("userId", c.Uuid))
	for {
		_, jsonMessage, err := c.Conn.ReadMessage()
		if err != nil {
			// 正常关闭不记录 Error
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				zap.L().Error("ws read error", zap.String("userId", c.Uuid), zap.Error(err))
			}
			return
		}

		// 安全：由服务端注入 send_id，防止 IDOR 攻击
		securedMessage := injectSenderId(jsonMessage, c.Uuid)

		if err := c.broker.Publish(ctx, securedMessage); err != nil {
			zap.L().Error("publish message error", zap.String("userId", c.Uuid), zap.Error(err))
		}
	}
}
```

### 4.2 Write：推送给前端并更新消息状态

```go
func (c *UserConn) Write() {
	ticker := time.NewTicker(pingPeriod) // 心跳 Ping
	defer ticker.Stop()

	zap.L().Info("ws write goroutine start", zap.String("userId", c.Uuid))
	for {
		select {
		case messageBack, ok := <-c.SendBack:
			if !ok {
				// SendBack 已关闭，发送 Close 帧后退出
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, messageBack.Message); err != nil {
				zap.L().Error("ws write error", zap.String("userId", c.Uuid), zap.Error(err))
				return
			}

			// 推送成功后更新消息状态
			if repo := c.broker.GetMessageRepo(); repo != nil {
				_ = repo.UpdateStatus(messageBack.Uuid, message_status.Sent)
			}

		case <-ticker.C:
			// 定期发送 Ping 心跳
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				zap.L().Info("ws ping failed, closing", zap.String("userId", c.Uuid), zap.Error(err))
				return
			}
		}
	}
}
```

---

## 5. MessageBroker 抽象与依赖注入

### 5.1 MessageBroker 接口

```go
type MessageBroker interface {
	Publish(ctx context.Context, msg []byte) error
	RegisterClient(client *UserConn)
	UnregisterClient(client *UserConn)
	GetClient(userId string) *UserConn
	KickClient(userId string, reason string)       // 单点登录互踢
	PushRecallNotify(messageUuid, receiveId string) // 撤回通知
	Start()
	Close()
	GetMessageRepo() mysql.MessageRepository
}
```

*文件位置：`internal/service/chat/server.go`*

### 5.2 main.go 组装与注入

项目在启动时创建 `ChatServer`，内部初始化 Kafka broker：

```go
chatServer := chat.NewChatServer(chat.ChatServerConfig{
	MessageRepo:     repos.Message,
	FriendshipRepo:  repos.Friendship,
	GroupMemberRepo: repos.GroupMember,
	SessionRepo:     repos.Session,
	CacheService:    cacheService,
	UserRepo:        repos.User,
})
chatServer.InitKafka()

handlers := handler.NewHandlers(services, chatServer.GetBroker())
go chatServer.Run()
```

*文件位置：`cmd/kama_chat_server/main.go`*

---

## 6. 连接升级与生命周期（ws_gateway）

### 6.1 建立连接：NewClientInit

```go
func NewClientInit(c *gin.Context, clientId string, broker MessageBroker) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		zap.L().Error(err.Error())
		return
	}
	client := &UserConn{
		Conn:        conn,
		Uuid:        clientId,
		SendBack:    make(chan *MessageBack, constants.CHANNEL_SIZE),
		broker:      broker,
	}
	broker.RegisterClient(client)
	go client.Read()
	go client.Write()
}
```

### 6.2 断开连接：ClientLogout

```go
func ClientLogout(clientId string, broker MessageBroker) error {
	client := broker.GetClient(clientId)
	if client != nil {
		client.cleanup() // 统一通过 sync.Once 释放资源
	}
	return nil
}
```

---

## 7. Broker 实现：Kafka 模式

### 7.1 Kafka 模式：MsgConsumer（kafka_broker.go）

关键点：

- `Publish()`：producer 写入 Kafka
- `Start()`：启动 goroutine 从 Kafka 消费消息，并对本机在线用户做推送
- 维护 `Clients sync.Map`，用于判断某用户是否在本机在线
- `KickClient()`：单点登录互踢，推送下线通知并断开连接
- `PushRecallNotify()`：本地直推撤回通知（非 Kafka）

缓存（异步）：通过注入的 `AsyncCacheService.SubmitTask()` 更新 Redis。

- 单聊缓存 key：`message:list:<userOne>_<userTwo>`（先按字符串大小排序）
- 群聊缓存 key：`message:group_list:<groupId>`

---

## 8. WebSocket Handler 与路由

### 8.1 路由

当前路由（以注册代码为准）：

- `GET /ws`：WebSocket 登录（升级连接）
- `POST /ws/logout`：WebSocket 登出

*文件位置：`internal/router/ws_routes.go`*

### 8.2 鉴权要求（非常重要）

`/ws` 与 `/ws/logout` 都注册在私有路由组中，会经过 `JWTAuth()`：

- 必须携带 Header：`Authorization: Bearer <access_token>`
- 用户 ID 从 JWT 上下文获取，不信任客户端传入的参数

*文件位置：`internal/router/router.go` 与 `internal/infrastructure/middleware/jwt_middleware.go`*

### 8.3 Handler

```go
type WsHandler struct {
	broker chat.MessageBroker
}

func (h *WsHandler) WsLoginHandler(c *gin.Context) {
	// 安全：从 JWT 上下文获取用户 ID，不信任客户端传入的参数
	userId, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusOK, gin.H{"code": errorx.CodeUnauthorized, "msg": "请先登录"})
		return
	}
	clientId := userId.(string)
	chat.NewClientInit(c, clientId, h.broker)
}

func (h *WsHandler) WsLogoutHandler(c *gin.Context) {
	// 安全：从 JWT 上下文获取用户 ID，不使用请求体
	userId, exists := c.Get("user_id")
	if !exists {
		HandleError(c, errorx.New(errorx.CodeUnauthorized, "请先登录"))
		return
	}
	if err := chat.ClientLogout(userId.(string), h.broker); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

*文件位置：`internal/handler/ws_handler.go`*

---

## 9. 消息类型与消息体

### 9.1 ChatMessageRequest

```go
type ChatMessageRequest struct {
    SessionId  string `json:"session_id"`
    Type       int8   `json:"type" binding:"required"`
    Content    string `json:"content"`
    Url        string `json:"url"`
    SendId     string `json:"send_id" binding:"required"`
    SendName   string `json:"send_name"`
    SendAvatar string `json:"send_avatar"`
    ReceiveId  string `json:"receive_id" binding:"required"`
    FileSize   string `json:"file_size"`
    FileType   string `json:"file_type"`
    FileName   string `json:"file_name"`
    AVdata     string `json:"av_data"`
}
```

*文件位置：`internal/dto/request/chat_message_request.go`*

### 9.2 Type 枚举值（以代码为准）

*文件位置：`pkg/enum/message/message_type/message_type.go`*

- `0`：Text（文本）
- `1`：Voice（语音）
- `2`：File（文件）
- `3`：AudioOrVideo（音视频通话）
- `4`：Recall（消息撤回）
- `99`：KickNotification（设备下线通知/单点登录互踢）

---

## 10. 消息流转全景（高层）

```
前端 WebSocket
    ↓
WsLoginHandler 升级连接（用户 ID 从 JWT 获取）
    ↓
UserConn.Read() 读消息 → injectSenderId 安全注入 send_id
    ↓
MessageBroker.Publish() → Kafka Producer
    ↓
Kafka Consumer 消费消息
    ↓
路由推送到目标 UserConn.SendBack
    ↓
UserConn.Write() 写回前端 + 更新消息状态
```

---

## 11. 测试与调试（按当前鉴权方式）

### 11.1 WebSocket 握手测试（推荐用 wscat/websocat）

因为后端要求 `Authorization: Bearer <access_token>`，浏览器原生 `new WebSocket(url)` 无法自定义该 Header。

示例（wscat）：

```bash
wscat -c 'ws://localhost:8000/ws' \
  -H 'Authorization: Bearer <ACCESS_TOKEN>'
```

### 11.2 发送消息示例（复制到 wscat 输入即可）

文本消息（`type=0`）：

```json
{
  "session_id": "S123456_654321",
  "type": 0,
  "content": "Hello, 这是一条测试消息",
  "send_id": "U123456",
  "send_name": "张三",
  "send_avatar": "/static/avatars/default.png",
  "receive_id": "U654321"
}
```

文件消息（`type=2`）：

```json
{
  "session_id": "S123456_654321",
  "type": 2,
  "url": "/static/files/document.pdf",
  "file_size": "1.2MB",
  "file_type": "pdf",
  "file_name": "重要文档.pdf",
  "send_id": "U123456",
  "send_name": "张三",
  "send_avatar": "/static/avatars/default.png",
  "receive_id": "U654321"
}
```

### 11.3 登出接口测试（Postman/ curl）

```bash
curl -X POST 'http://localhost:8000/ws/logout' \
  -H 'Authorization: Bearer <ACCESS_TOKEN>'
```

成功响应（以项目统一响应为准）：

```json
{
  "code": 1000,
  "msg": "success",
  "data": null
}
```

---

## 12. 常见问题与最佳实践

### 12.1 消息处理慢/堆积怎么办？

当前实现中，`UserConn.Read()` 直接把消息交给 broker，通过 Kafka Producer 写入。

常见处理方向：

1. 调整 `constants.CHANNEL_SIZE`
2. 优化消费侧逻辑（数据库写入、群成员查询、缓存更新）
3. 增加 Kafka 分区或消费者实例（水平扩展）

---

## 13. 本章小结

- WebSocket 网关在 `ws_gateway.go`：负责升级连接与读写协程
- 消息路由通过 `MessageBroker` 抽象解耦，当前仅支持 Kafka 模式
- `/ws` 与 `/ws/logout` 处于 JWT 私有路由，需要 `Authorization: Bearer <access_token>`
- 安全机制：用户 ID 从 JWT 获取，消息发送时由服务端注入 send_id 防止 IDOR 攻击

下一章将进入消息处理细节（单聊/群聊的入库、路由与缓存更新）。
