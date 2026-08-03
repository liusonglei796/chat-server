# 数据库拆分三步改造设计（单服务单库）

日期: 2026-08-03
状态: 已确认（用户批准方向，进入实施计划阶段）

## 背景与动机

当前 chat-server 的 4 个微服务（user/auth/relation/message）+ 网关（chat_server）共享同一个 MySQL 实例、同一个 `chat` 库、同一套 7 张表（user, group, friendship, session, apply, message, group_member），并且所有服务用同一个 root 账号连接。这是 Shared Database 反模式：

- user 表被 4 个服务读写，session 表被 4 个服务写
- 表结构是服务间隐式契约，schema 变更需全体协调发布
- 无法独立扩容/备份/授权

目标：按生产实践（microservices.io Database per Service + API Composition + 事件驱动 + Outbox）分三步改造为"单服务单库"。

## 目标架构

```
                 浏览器/App
                    │ HTTP/WS
                    ▼
              chat_server (网关：HTTP + WS + Kafka)
                    │ gRPC 读（跨服务读全部 API 化）
        ┌───────────┼───────────────┐
        ▼           ▼               ▼
  user_service  relation_service  message_service
  (chat_user库)  (chat_relation库) (chat_message库)
  user 表         friendship/group    session/message
                 /group_member/apply
        └───────────┼───────────────┘
                    │ Kafka 事件（写路径全部事件化）
              domain_events topic
        ┌───────────┼───────────────┐
        │     每服务本地 Outbox 表     │  ← 事务内写业务+outbox，发布器异步投递
```

## 数据库归属（不可变更的契约）

| 库 | 账号 | 表 | 独占服务 |
|---|---|---|---|
| `chat_user` | `svc_user` | user | user_service |
| `chat_relation` | `svc_relation` | friendship, group, group_member, apply | relation_service |
| `chat_message` | `svc_message` | session, message | message_service |

- 每个服务仅能访问自己的库（通过 GRANT 强制）
- auth_service 无自有表，并入 user_service（其 Login/Register/Logout/ValidateTokenID/GetUserIsAdmin 全部成为 user_service 的 gRPC 方法）
- 网关 chat_server 不直连任何 MySQL（现状已满足）

## 第 1 步：逻辑隔离

### 1.1 建库与账号

新增 `migrations/split_db.sql`：

```sql
-- 建库
CREATE DATABASE IF NOT EXISTS chat_user CHARACTER SET utf8mb4;
CREATE DATABASE IF NOT EXISTS chat_relation CHARACTER SET utf8mb4;
CREATE DATABASE IF NOT EXISTS chat_message CHARACTER SET utf8mb4;

-- 账号
CREATE USER IF NOT EXISTS 'svc_user'@'%' IDENTIFIED BY 'svc_user_pwd';
CREATE USER IF NOT EXISTS 'svc_relation'@'%' IDENTIFIED BY 'svc_relation_pwd';
CREATE USER IF NOT EXISTS 'svc_message'@'%' IDENTIFIED BY 'svc_message_pwd';

-- 授权（每个账号仅自己的库）
GRANT ALL PRIVILEGES ON chat_user.* TO 'svc_user'@'%';
GRANT ALL PRIVILEGES ON chat_relation.* TO 'svc_relation'@'%';
GRANT ALL PRIVILEGES ON chat_message.* TO 'svc_message'@'%';
FLUSH PRIVILEGES;
```

### 1.2 配置

`configs/config.toml` 与 `configs/config_local.toml` 的 `[mysqlConfig]` 拆分为按服务区分：
- user_service / auth 并入后：databaseName = `chat_user`
- relation_service：databaseName = `chat_relation`
- message_service：databaseName = `chat_message`

每个服务 `main.go` 的 `mysqlimpl.Init()` 改为读取自己的库名。实现方式：新增 `MysqlConfig.DatabaseName` 通过启动参数或环境变量（`DB_NAME`）覆盖，保持单个配置文件不变。

### 1.3 AutoMigrate 收敛

每个服务 `main.go` 中 `db.AutoMigrate(...)` 只迁移自己库的表：
- user_service: `&model.UserInfo{}`
- relation_service: `&model.GroupInfo{}, &model.Friendship{}, &model.Apply{}, &model.GroupMember{}`
- message_service: `&model.Session{}, &model.Message{}`

### 1.4 数据迁移

新增 `migrations/split_migrate.sql`（一次性，从旧 chat 库分发数据）：
- `INSERT INTO chat_user.user SELECT * FROM chat.user`
- `INSERT INTO chat_relation.friendship SELECT * FROM chat.friendship`（同理 group/group_member/apply）
- `INSERT INTO chat_message.session/message SELECT * FROM chat.session/message`
- 迁移前需停服或接受短暂不一致；迁移脚本在部署流程中执行一次

### 1.5 docker-compose

mysql 容器挂载 `migrations/split_db.sql` 到 `/docker-entrypoint-initdb.d/`（首次启动自动执行建库建账号）。服务 DSN 改为各自库与账号。

## 第 2 步：跨服务读 API 化（gRPC）

### 2.1 新增 proto 方法

`api/proto/user.proto` 新增：

```proto
// 批量获取公开用户信息（昵称/头像），用于列表页避免 N+1 跨服务调用
rpc BatchGetPublicUserInfo(BatchGetPublicUserInfoRequest) returns (BatchGetPublicUserInfoResponse);

message BatchGetPublicUserInfoRequest {
  repeated string user_ids = 1;
}
message BatchGetPublicUserInfoResponse {
  repeated PublicUserInfo users = 1;
}
```

`PublicUserInfo` 复用已有结构（uuid/nickname/avatar/gender/birthday/signature）。

`api/proto/relation.proto` 新增（供 message/session 服务校验用）：

```proto
// 检查好友关系状态（NORMAL/BLACK/BE_BLACK/不存在）
rpc CheckFriendship (CheckFriendshipRequest) returns (CheckFriendshipResponse);
message CheckFriendshipRequest { string user_id = 1; string friend_id = 2; }
message CheckFriendshipResponse { int32 status = 1; }  // 0=非好友 1=正常 2=已拉黑对方 3=被对方拉黑

// 检查用户是否群成员
rpc CheckGroupMember (CheckGroupMemberRequest) returns (CheckGroupMemberResponse);
message CheckGroupMemberRequest { string group_id = 1; string user_id = 2; }
message CheckGroupMemberResponse { bool is_member = 1; }
```

重新生成 `api/gen/`（protoc 生成 *_grpc.pb.go 与 *.pb.go），实现侧复用现有 `FindByUuids` / `FindByUserIdAndFriendId` / `FindByGroupAndUser` 查询。

### 2.2 各服务读路径替换（按实测访问矩阵）

**前置发现**：消息列表（`message.SendName/SendAvatar`）与会话列表（`session.ReceiveName/Avatar`）均为**写入时冗余快照**，读取时不实时查 user/group 表——因此 R3/R4/R5 无需 gRPC 改造，真正需要改造的是"写冗余快照时"与"校验型查询"两类点。

| # | 现状（直查共享库） | 改为 | 涉及文件 |
|---|---|---|---|
| R1 | relation 查 user 状态（ApplyFriend/PassFriendApply 校验） | gRPC `GetUserInfo`（现有接口已返回 `status` 字段，直接用，不新增） | internal/service/apply/service.go, internal/service/friendship/service.go |
| R1b | **message/session 服务读 user/group/friendship/group_member 表**（CreateSession/OpenSession/CheckOpenSessionAllowed 校验 sendId/receiveId 存在性、好友关系、群成员身份、目标状态） | gRPC：user 状态→`GetUserInfo`；群信息→relation `GetGroupDetail`；**好友关系→relation 新增 `CheckFriendship`**；**群成员→relation 新增 `CheckGroupMember`** | internal/service/session/service.go |
| R2 | relation 好友列表/群成员列表/申请列表需昵称头像（cacheHelper.GetOrLoad 逐个查 user） | 列表方法改为"查本地关系表 → 收集 userIds → 一次 gRPC `BatchGetPublicUserInfo` → 组装返回"；缓存层（cacheHelper/singleflight）本轮保留用于 GetUserInfo 单点查询，批量路径可后续演进 | internal/service/friendship/service.go, internal/service/group/service.go, internal/service/apply/service.go |
| R3 | message 消息列表 sendName/sendAvatar | **无需改造**（写入时冗余快照，见前置发现） | internal/service/message/service.go |
| R4 | message 会话列表需用户名/头像 | **无需改造**（session 表冗余 ReceiveName/Avatar） | internal/service/session/service.go |
| R5 | message 群会话列表需群信息 | **无需改造**（群会话列表用 session.ReceiveName 冗余） | internal/service/session/service.go |
| R6 | user_service 写 session 表（KickUser/UpdateUserInfo） | 移除直写，改发事件（第 3 步） | internal/service/user/service.go |
| R7 | auth_service 持有 user 表逻辑 | 全部并入 user_service gRPC | cmd/auth_service/*, internal/service/auth/* |

### 2.3 auth_service 并入 user_service

- user_service 的 GrpcServer 增加：Login / Register / Logout / ValidateTokenID / GetUserIsAdmin（业务逻辑复用现有 `user.UserService` 与 `auth.Service`，auth.Service 仅依赖 cache+userRepo，可并入 user 服务构造）
- 网关 chat_server 的 `grpc_client` 中 AuthClient 改为指向 user_service（`etcd://user_service`），或直接改用 UserClient 新方法
- 删除 `cmd/auth_service/`、`internal/service/auth/grpc_server.go`（或保留 auth.Service 作为 user 服务内部组件），docker-compose 移除 auth-service 容器
- 前端/网关 API 路由不变（HTTP 接口签名不动）

### 2.4 新增 relation_service 的批量群信息方法（如需）

若 R5 需要，在 `api/proto/relation.proto` 增加 `BatchGetGroupInfo`；若现有群会话列表逻辑可直接用已有 `GetGroupDetail` 逐个调，则优先复用，避免新增接口。

## 第 3 步：写路径事件化 + Outbox

### 3.1 Outbox 表（每库一张，归属该库服务）

```sql
CREATE TABLE IF NOT EXISTS outbox (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  event_type VARCHAR(64) NOT NULL,      -- friend_approved / group_joined / user_updated ...
  payload JSON NOT NULL,                -- 事件体
  status TINYINT NOT NULL DEFAULT 0,    -- 0=pending 1=sent
  attempt INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  sent_at DATETIME NULL,
  KEY idx_status_created (status, created_at)
);
```

### 3.2 新增组件

- `pkg/outbox/`：
  - `OutboxRepo`：写入（在业务事务内）/ 查询 pending / 标记 sent
  - `Publisher`：后台 goroutine 定时（默认 1s）轮询 pending 事件 → 发 Kafka `domain_events` → 标记 sent；失败重试（attempt++），指数退避，超过最大尝试数进入死信（status=2）
  - 每服务启动时拉起自己的 Publisher
- `internal/dto/event/`：新增业务事件结构（复用现有 push_event.go 的模式）：
  - `FriendApprovedEvent`（applicant_id, user_id, session 相关）
  - `GroupJoinedEvent` / `GroupMemberRemovedEvent`
  - `UserUpdatedEvent`（user_id, nickname, avatar）
  - `FriendRemovedEvent`
- Kafka 新增 topic：`domain_events`
- 幂等消费：consumer 端用 Redis SETNX（key=`outbox:dedup:<event_type>:<业务id>`，TTL 24h）去重。选 Redis 而非唯一索引的原因：session 表无天然业务唯一键（uuid 为雪花生成，重复消费会插两条），Redis SETNX 对所有事件类型通用且无需改表

### 3.3 写路径拆分清单

**实测修正**：PassFriendApply（仅 apply+friendship）、DeleteFriend（仅 friendship+apply）、KickUser（仅 Redis token）均**不写 session 表**，无需事件化——只需 R1 的 gRPC 读替换。真正跨库写 session 的只有以下 6 处：

| # | 现状（relation/user 服务写 session 表） | 改为 | 归属服务 |
|---|---|---|---|
| W1 | CreateGroup：group+member+**session** 一个事务 | 事务(group+member+outbox `group_created`) → message 消费建群主会话 | relation → message |
| W2 | ApplyGroup(DIRECT)：member+count+**session** | 事务(member+count+outbox `group_joined`) → message 消费建成员会话 | relation → message |
| W3 | PassGroupApply：apply+member+count+**session** | 事务(apply+member+count+outbox `group_joined`) → message 消费建成员会话 | relation → message |
| W4 | DismissGroup：删成员+删群+**session** | 事务(删成员+删群+outbox `group_dismissed`) → message 消费软删群会话 | relation → message |
| W5 | UpdateGroupInfo：改 group+**session 冗余** | 事务(改 group+outbox `group_updated`) → message 消费更新会话冗余字段 | relation → message |
| W6 | UpdateUserInfo：改 user+**session 冗余** | 事务(改 user+outbox `user_updated`) → message 消费更新会话冗余字段 | user → message |

事件类型全集：`group_created` / `group_joined` / `group_dismissed` / `group_updated` / `user_updated`

### 3.4 一致性保证

- Outbox 保证"业务成功 = 事件一定最终发出"（业务与 outbox 同事务）
- 消费幂等（Redis SETNX 或唯一索引）保证 at-least-once 下的正确性
- 补偿：本设计无需要撤销的流程（失败重试即可），不引入 Saga（见设计决策）

## 设计决策记录

1. **auth_service 并入 user_service**（用户确认）：auth 无自有表，Login/Register 本就操作 user 表，并入后每服务有且仅有自己的数据。
2. **不引入 Saga**：所有跨服务写均为"接力推进，失败重试即可"，无需要整体撤销的流程。Saga 是 YAGNI。
3. **逻辑隔离而非独立实例**：单机部署下同实例多库 + 独立账号 + GRANT 是 microservices.io 推荐的 FTGO 做法，后续可平滑演进为独立实例。
4. **Outbox 轮询式发布器**（1s 间隔）而非 CDC：当前规模下轮询足够，CDC（debezium）是后续演进项。
5. **读路径优先 gRPC 同步**，高频列表用 `BatchGetPublicUserInfo` 避免 N+1；数据复制（事件同步副本）是后续演进项（第 4 步），不在本次范围。

## 验证标准

1. `go build ./...` 通过，无未处理错误
2. `docker-compose up -d` 全链路启动成功（mysql 初始化建库建账号、5 个业务容器改 4 个：chat-server/user/relation/message）
3. 各服务仅能访问自己的库（用错误库名账号连接验证被拒）
4. `test_all_apis.py` 全量 API 回归通过（登录/好友/群/消息/会话全链路）
5. 手动验证：加好友 → 会话创建成功（事件链路通）；改昵称 → 会话显示名更新（事件链路通）
6. Outbox 验证：模拟 Kafka 不可用时业务仍成功（事件 pending），恢复后自动补发
7. 压测（如用户要求）：按既定规范输出 RPS/P99/Tempo trace

## 范围外（后续演进，不在本次）

- 独立 MySQL 实例/容器
- CDC 发布器（debezium）
- 数据复制读模型（事件同步用户副本到 message/relation）
- Saga 编排器
- 分库分表
