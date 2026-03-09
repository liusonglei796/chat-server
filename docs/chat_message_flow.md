# 聊天消息完整流程

## 整体架构概览

```mermaid
graph LR
    A["前端浏览器"] <-->|WebSocket| B["ws_gateway.go"]
    B -->|Publish| C["kafka_broker.go"]
    C -->|Producer 写入| E["Kafka 集群"]
    E -->|Consumer 读取| D2["kafka_client.go"]
    D2 -->|ReadMessage| C2["kafka_broker.go"]
    C2 -->|SendBack channel| B
```

---

## 一、上行链路：用户发送消息 → Kafka

> 前端发出的 JSON 原封不动地写入 Kafka，**不做任何加工**。

```mermaid
sequenceDiagram
    participant 前端 as 前端浏览器
    participant WS as ws_gateway.go<br/>Read()
    participant Broker as kafka_broker.go<br/>Publish()
    participant Kafka as Kafka 集群

    前端->>WS: WebSocket 发送 JSON<br/>(ChatMessageRequest)
    Note over WS: Conn.ReadMessage()<br/>读取原始 []byte
    WS->>Broker: broker.Publish(ctx, jsonMessage)<br/>原始 JSON 透传
    Note over Broker: key = []byte("0")<br/>固定分区 Key<br/>Producer.WriteMessages()<br/>封装为 kafka.Message
    Broker->>Kafka: 写入 Topic
```

### 涉及的代码

| 步骤 | 文件 | 方法 | 作用 |
|------|------|------|------|
| 1 | `ws_gateway.go` | `Read()` | `Conn.ReadMessage()` 阻塞读取 WebSocket 消息 |
| 2 | `ws_gateway.go` | `Read()` | `c.broker.Publish()` 将原始 JSON 传递给 broker |
| 3 | `kafka_broker.go` | `Publish()` | 设置 `key=[]byte("0")`，封装为 `kafka.Message{Key, Value}` 并写入 Kafka |

---

## 二、下行链路：Kafka → 消费 → 推送给用户

> 消息的**所有加工**都发生在这个阶段。

```mermaid
sequenceDiagram
    participant Kafka as Kafka 集群
    participant Client as kafka_client.go<br/>Consumer
    participant Start as kafka_broker.go<br/>Start() 消费循环
    participant Handle as kafka_broker.go<br/>handleXxxMessage()
    participant Dispatch as kafka_broker.go<br/>dispatchToUser/Group()
    participant Write as ws_gateway.go<br/>Write()
    participant 前端 as 前端浏览器

    Kafka->>Client: Consumer.ReadMessage()<br/>阻塞读取
    Client->>Start: 返回 kafka.Message
    Note over Start: 1. json.Unmarshal<br/>反序列化为 ChatMessageRequest
    Start->>Handle: 按 Type 分发<br/>Text / File / AV

    Note over Handle: 2. checkSendPermission()<br/>权限校验(好友/群成员/禁用)
    Note over Handle: 3. buildMessageFromRequest()<br/>生成 SnowflakeID + 构建 Model
    Note over Handle: 4. persistMessage()<br/>持久化到 MySQL
    Note over Handle: 5. updateSessionLastMessage()<br/>异步更新会话摘要

    Handle->>Dispatch: 路由到 User 或 Group
    Note over Dispatch: 6. json.Marshal 构造响应 DTO<br/>(GetMessageListRespond)
    Note over Dispatch: 7. 写入 SendBack channel<br/>(trySendBack 非阻塞)
    Note over Dispatch: 8. 异步更新 Redis 缓存

    Dispatch->>Write: SendBack channel ← MessageBack
    Note over Write: Conn.WriteMessage()<br/>推送到 WebSocket
    Write->>前端: 接收到消息 JSON
```

### 涉及的代码

| 步骤 | 文件 | 方法 | 作用 |
|------|------|------|------|
| 1 | `kafka_broker.go` | `Start()` L140 | `Consumer.ReadMessage()` 从 Kafka 拉取消息 |
| 2 | `kafka_broker.go` | `Start()` L158 | `json.Unmarshal` 反序列化为 `ChatMessageRequest` |
| 3 | `kafka_broker.go` | `Start()` L165 | 按 `Type` 分发到 `handleText/File/AV` |
| 4 | `kafka_broker.go` | `handleTextMessage()` L454 | `checkSendPermission()` 权限校验 |
| 5 | `kafka_broker.go` | `handleTextMessage()` L461 | `buildMessageFromRequest()` 构建消息 Model |
| 6 | `kafka_broker.go` | `handleTextMessage()` L470 | `persistMessage()` 持久化 MySQL |
| 7 | `kafka_broker.go` | `dispatchToUser()` L641 | `json.Marshal` 序列化响应 DTO |
| 8 | `kafka_broker.go` | `dispatchToUser()` L657 | `trySendBack()` 写入 `SendBack` channel |
| 9 | `ws_gateway.go` | `Write()` L162 | `Conn.WriteMessage()` 推送到浏览器 |

---

## 三、私聊 vs 群聊 分发差异

```mermaid
graph TD
    A["handleTextMessage()"] --> B{ReceiveId 首字母?}
    B -->|U 开头 → 私聊| C["dispatchToUser()"]
    B -->|G 开头 → 群聊| D["dispatchToGroup()"]

    C --> C1["推送给接收者 SendBack"]
    C --> C2["回显给发送者 SendBack"]
    C --> C3["更新私聊 Redis 缓存"]

    D --> D1["查询群成员列表<br/>(优先 Redis 缓存)"]
    D1 --> D2["遍历成员推送 SendBack<br/>(排除发送者)"]
    D --> D3["回显给发送者 SendBack"]
    D --> D4["更新群聊 Redis 缓存"]
```

---

## 四、关键设计要点

| 设计点 | 说明 |
|--------|------|
| **上行透传** | 前端 JSON 原封不动写入 Kafka，不做序列化/反序列化 |
| **下行加工** | 所有业务逻辑（校验、持久化、路由）集中在消费端 |
| **读写分离** | 每个 WebSocket 连接有独立的 `Read` 和 `Write` goroutine |
| **非阻塞推送** | `trySendBack` 使用 `select-default` 防止 channel 满时阻塞消费者 |
| **心跳保活** | Ping/Pong 机制（54秒发 Ping，60秒 Pong 超时）检测死连接 |
| **缓存优先** | 群成员查询使用 Cache-Aside + singleflight 防击穿 |
