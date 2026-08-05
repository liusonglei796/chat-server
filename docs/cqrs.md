# CQRS 模式解析

> 本文档解释 CQRS（Command Query Responsibility Segregation，命令查询职责分离）模式，
> 并结合本项目（chat-server）微服务拆分后的实际落地场景说明：
> **为什么聊天系统需要 CQRS**，以及**本项目是怎么用"事件 + 读模型"实现它的**。

---

## 1. 什么是 CQRS

CQRS 的核心思想一句话：**读和写是两套不同的模型，不该共用同一个数据入口。**

- **Command（命令/写）**：改变状态的操作。`建群`、`发消息`、`同意加好友`。
- **Query（查询/读）**：不改变状态的操作。`拉取会话列表`、`获取群成员`。

传统 CRUD 里，读写共用同一个实体模型和同一条数据链路——`userRepository.FindByUuid` 既能服务于
"用户更新资料"（写），也能服务于"展示用户信息"（读）。单体应用里这没问题，因为一切都在本地、一个数据库。

但**微服务拆分之后，读写共用模型的假设崩塌了**，于是需要 CQRS。

---

## 2. 为什么聊天系统需要它：微服务拆分后的两个痛点

本项目按业务拆成了多个服务（user / group / friendship / apply / message / chat），**每个服务持有自己的数据库**。
这带来两个直接后果：

### 痛点一：跨服务查询的成本

一个服务想读"另一个服务的表"时，没有本地数据库可以查。本项目的规定是：

> **跨库读 = gRPC 调用；跨库写 = 事件（outbox + Kafka）。**

例如 message 服务要判断"发消息的人是否被禁用 / 是不是好友 / 是不是群成员"，只能调 gRPC：

```go
// internal/apps/message/message/kafka_processor.go
senderStatus, err := grpc_client.GetUserStatus(ctx, sendId)         // → user 服务
fsStatus, err := grpc_client.CheckFriendshipStatus(ctx, sendId, receiveId) // → friendship 服务
isMember, err := grpc_client.CheckGroupMember(ctx, receiveId, sendId)      // → group 服务
```

如果每次查询都走 gRPC，接口延迟高、依赖链长、一挂全挂。

### 痛点二：本地表需要"别人的数据"来展示

会话列表要显示群名、群头像、对方昵称——这些数据在 group / user 服务的库里。
message 服务的 `session` 表于是**冗余**了这些字段：

```go
// internal/common/model/session.go
ReceiveName string `gorm:"column:receive_name;..."` // 冗余的群名/用户昵称
Avatar      string `gorm:"column:avatar;..."`       // 冗余的头像
```

冗余必然面临一个问题：**群改名了、用户换头像了，message 服务本地这份拷贝怎么更新？**

---

## 3. CQRS 的答案：读模型（Read Model）与事件投影

CQRS 把这层冗余数据称为**读模型（Read Model）**：

- **写模型（Command Side）**：各服务的"主数据"——group 表、user 表，是数据的唯一真相源（source of truth）。
- **读模型（Query Side）**：为查询优化的冗余副本——message 服务的 session 表，只为快速展示而生。

读模型不直接写，而是**通过订阅事件被动更新**。群改名了，group 服务发一个 `group_updated` 事件，
message 服务收到后更新本地 session 表——这个过程叫**投影（Projection）**。

```
                     ┌─────────────┐
  群改名（group服务）→ │ outbox 表    │ ──轮询──> Kafka domain_events
                     └─────────────┘                 │
                                                      ▼
                                          message 服务事件消费者
                                                      │
                                                      ▼
                                          session 表（读模型）更新
```

### 本项目中的投影：`session_event_handler.go`

message 服务的 `SessionEventHandler` 就是投影器——它把各种领域事件翻译成本地 session 表操作：

```go
// internal/apps/message/message/session_event_handler.go
func (h *SessionEventHandler) Handle(ctx context.Context, eventType string, payload []byte) error {
	switch eventType {
	case event.EventGroupCreated:   // 建群 → 创建群会话
		return h.createGroupSession(ctx, e.GroupId, e.OwnerId, e.GroupName, e.GroupAvatar)
	case event.EventGroupUpdated:   // 群改名/换头像 → 更新本地冗余字段
		return h.sessionStore.UpdateByReceiveId(ctx, e.GroupId, updates)
	case event.EventUserUpdated:    // 用户改昵称/头像 → 更新本地冗余字段
		return h.sessionStore.UpdateByReceiveId(ctx, e.UserId, updates)
	case event.EventFriendBlacked:  // 拉黑 → 软删双方私聊会话
		return h.softDeleteFriendSessions(ctx, e.UserId, e.FriendId)
	}
	return nil
}
```

---

## 4. 为什么一个服务要消费多种事件

读模型的本质是"多个写模型的投影"——message 服务的 session 表同时投影了 group 的群信息、user 的用户信息、
friendship 的好友关系。所以**一个服务的读模型往往需要多个事件流**：

| 事件 | 来源服务 | message 服务的投影动作 |
|---|---|---|
| `group_created` | group | 创建群会话 |
| `group_joined` | group | 创建群会话 |
| `group_dismissed` | group | 软删群会话 |
| `group_updated` | group | 更新冗余的群名/头像 |
| `user_updated` | user | 更新冗余的昵称/头像 |
| `friend_blacked` | friendship | 软删双方私聊会话 |

这就是为什么事件消息上必须带 `event_type`：**同一 topic 里混着多种事件结构，消费者必须靠类型
选对反序列化目标**。这也是"一个服务只对应一种事件"直觉的错误之处——**读模型的投影源是多样的**。

---

## 5. 事件的可靠性：outbox 模式

投影依赖事件，事件不能丢。本项目用 **outbox 模式**保证"本地事务 + 事件发布"的原子性：

```
业务事务（本地库）              异步发布
┌──────────────────────┐    ┌──────────────────────┐
│ 1. 改群名             │    │ 轮询 outbox 表         │
│ 2. 插入 outbox 记录   │ ──> │ 发到 Kafka domain_events│
│   （同一事务，同生共死）│    │ 成功 → 标记已发布       │
└──────────────────────┘    └──────────────────────┘
```

- 事件先写**本地** outbox 表（和业务数据同一事务），不直接发 Kafka。
- 后台 `outbox.Publisher` 轮询待发布记录，发到 Kafka，成功后标记。
- 发布失败则重试计数 +1，下次轮询再发——**不丢事件**。

```go
// internal/common/infrastructure/outbox/publisher.go
for _, e := range events {
	if err := p.publishFn(ctx, e.EventType, e.Uuid, []byte(e.Payload)); err != nil {
		_ = p.outboxStore.IncrementRetry(ctx, e.Uuid) // 失败：重试计数 +1
		continue
	}
	published = append(published, e.Uuid)             // 成功：待标记
}
```

---

## 6. 完整链路：一次"同意加群"背后发生了什么

以用户被同意加入群为例，串起整个 CQRS + 事件流：

```
1. 用户在 apply 服务提交加群申请
2. apply 服务同意申请：
   ├─ 本地事务：更新申请状态 + 写 outbox（event: group_apply_passed）
   └─ 提交事务（事件已安全落库）
3. outbox.Publisher 轮询到记录，发到 Kafka domain_events
4. group 服务消费者收到 group_apply_passed：
   ├─ 本地事务：插入群成员 + 递增成员数
   └─ 再写 outbox（event: group_joined）
5. outbox.Publisher 再次发布 group_joined
6. message 服务消费者收到 group_joined：
   └─ 投影：为新人创建群会话（读模型更新）
```

每个服务的**主数据（写模型）由本服务自己改**，**别人的副本（读模型）靠事件投影**。
没有任何服务能直接改别人的表——这就是 CQRS 在微服务下的终极形态。

---

## 7. 常见问题

### Q1: CQRS 是不是所有查询都得走读模型？

不是。CQRS 的粒度自己定。本项目中：
- **会话列表/群会话**（读多、需展示他人数据）→ 读模型投影。
- **本服务主数据的简单查询** → 直接查自己的表，不需要额外读模型。

### Q2: 读模型冗余，为什么不直接每次 gRPC 现查？

gRPC 现查的代价：每次会话列表渲染要并发调 group/user 服务、依赖链长、故障放大。
读模型把"查询时的高成本"转成"事件到达时的低成本投影"，**用异步换取查询的确定性**。

### Q3: 读模型会不会不一致（最终一致性）？

会，这是 CQRS 的固有取舍。事件从发出到投影完成有延迟，期间读模型是旧数据。
本项目接受**最终一致性**：聊天场景里"群名更新延迟几毫秒"完全可接受，换来的是查询路径的稳定和低延迟。

### Q4: 事件重复了怎么办？

Kafka 至少一次投递 + 投影消费可能重放。本项目投影动作（建会话/更新冗余字段/软删）都是
**幂等**的：会话已存在则跳过、更新同值无副作用、软删不存在则忽略。

---

## 8. 相关代码文件索引

| 文件 | 职责 |
|---|---|
| `internal/apps/message/message/session_event_handler.go` | 读模型投影器：事件 → session 表操作 |
| `internal/apps/{group,friendship,message}/domain_event_consumer.go` | 各服务领域事件消费者 |
| `internal/common/infrastructure/outbox/publisher.go` | outbox 轮询发布器 |
| `internal/common/infrastructure/kafka/kafka.go` | Kafka 统一封装（主题/生产/消费/Publish） |
| `internal/common/dto/event/domain_event.go` | 领域事件 DTO 与类型常量 |
| `internal/common/model/session.go` | 读模型表（含冗余的 receive_name/avatar） |

---

## 9. 一句话总结

> **CQRS = 把"改数据"和"读数据"拆开。微服务下，改数据（写模型）归各服务自己管，
> 别人的读模型靠事件投影维护——本项目的 session 冗余表就是读模型，
> outbox + Kafka 就是投影的管道。**
