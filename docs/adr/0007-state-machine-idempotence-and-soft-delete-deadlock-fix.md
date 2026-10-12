# 0007. 第三代状态机幂等模型落地与会话软删除死锁修复

- **状态**: Accepted
- **日期**: 2026-10-10

## 上下文
在系统完成纯 gRPC + Envoy 单协议架构升级后，针对写操作的幂等性防护与底层存储机制进行了全景审计，发现了两项关键缺陷：
1. **核心创建接口裸奔**：群聊创建 (`POST /groups`) 每次无条件生成新的雪花 ID，在 Envoy 超时自动重试或网络抖动时会导致重复建群。
2. **软删除与唯一键冲突死锁**：会话表 (`session`) 内嵌 `gorm.Model`（带软删除），但联合唯一键为 `(send_id, receive_id)`（未包含 `deleted_at`）。当用户退群软删除会话后再次进群时，GORM 过滤软删除记录导致判定不存在，而插入新会话时又触发 MySQL 唯一键冲突，回退查询仍因软删除查出空指针，导致用户永久无法重新打开会话（`ErrServerBusy`）。
3. **缺乏通用深模块**：之前各业务零散手写 Redis 互斥锁，单纯加锁无法抵御延迟网络重试，且业务代码重复。

## 决策
1. **封装深模块 `pkg/idempotent`（第三代状态机令牌）**：
   - 暴露精简通用接口 `Guard` 与泛型 `ExecuteJSON`。
   - 内部封装 Redis Lua 原子脚本：
     - **防连击**：`PROCESSING` 阶段授予 5 秒短租约，同一窗口内拦截并发连击；若上一实例崩溃，超时后自动租约自愈（Lease Healing）。
     - **防重试**：`SUCCESS` 阶段持久化成功返回值快照（保留 24 小时），后续网络重试到达时 0ms 秒级回放（Replay），不重复执行业务事务。
     - **持有者验证**：写回终态时校验 `ownerID`，防止慢查询过期后覆写新租约数据。
     - **时钟防漂移**：统一采用 Redis `TIME` 指令获取当前时间。
     - **平滑降级**：Redis 故障时记录警报并降级放行业务执行，保障系统整体可用性。
2. **彻底修复会话软删除死锁**：
   - 在 `SessionStore` 引入 `FindBySendIdAndReceiveIdUnscoped` 查询能力。
   - 在 `CreateSession` 遇到 `IsDuplicateKey` 冲突时，通过 `Unscoped()` 自动检测并清除历史记录的 `deleted_at` 标记，使退群/退会话的用户能够无缝复活会话。
3. **为群聊创建 (`POST /groups`) 接入幂等保护**：
   - `api/proto/group.proto` 中的 `CreateGroupRequest` 增加 `client_token` 字段，`CreateGroupResponse` 增加 `group_id` 回传。
   - gRPC Server 自动从请求体 `req.ClientToken` 或 RPC Metadata `X-Idempotency-Key` 提取防重凭证，经由 `idempotent.Guard` 保护。若重复请求到达，直接回显原群号。
4. **用户注册与凭据颁发职责解耦（方案一）**：
   - **安全铁律（严禁对 Token 进行幂等回放）**：`POST /auth/login` 作为安全敏感凭据接口，必须每次现场执行密码哈希与签名校验，签发独立生命周期的新 Token，严禁对 Token 进行缓存与回放。
   - **注册接口解耦与回显**：`POST /auth/register` 解耦为纯粹的用户实体创建操作，响应体中移除 `access_token` 与 `refresh_token`。接入 `idempotent.Guard`，当网络超时重试时，0ms 秒级回显首次注册生成的完整用户档案，彻底解决用户重试注册时尴尬报错“用户名已被占用”的体验缺陷。
5. **消息发送上行链路 (`SendMessage`) 统一升级接入**：
   - 将原手写 `SetNX` + `Get` 两次网络交互的旧版防重逻辑彻底退役，升级为统一的 `pkg/idempotent.Guard`。
   - 依赖 Lua 原子探针将检查与抢占合并为 1 次原子网络往返，同时获得 5 秒租约崩溃自愈与 Fencing Token 防覆写保障。
   - 重试请求 0ms 瞬间回显落库单号，防止弱网抖动引发消息重复写入数据库与重复多端推送广播。
   - gRPC Server 同步支持从请求体 `req.ClientMsgId` 或 RPC Metadata `X-Idempotency-Key` 双通道提取防重凭证。
6. **好友关系管理 (`BlackFriend` / `UnblackFriend`) 升级为数据库原子 CAS**：
   - 彻底废除原有“内存 Check + 数据库盲目 Overwrite”的弱并发模式，在 `FriendshipStore` 引入 `UpdateStatusWithExpected`。
   - 底层 SQL 严格下推原子 Compare 条件：`WHERE user_id = ? AND friend_id = ? AND status = ?`，并严格依赖 `RowsAffected > 0` 进行排他胜出判定。
   - 保证仅当状态机原子跃迁成功时才允许向 Outbox 表写入领域事件，彻底消除了弱网重试/并发连击导致向下游 Kafka 重复派发 `friend_blacked` 事件的副作用扩散隐患。
7. **单向状态机 CAS 终态回显自愈 (Idempotent Healing)**：
   - **审批链路 (`apply_service`)**：针对 `PassFriendApply`, `PassGroupApply`, `RefuseFriendApply`, `RefuseGroupApply`, `BlackFriendApply`, `BlackGroupApply`，在执行 CAS 原子更新前与 CAS 失败后执行终态检查。若当前申请单已达成目标终态（如已是 AGREE/REFUSE/BLACK），判定为网络超时重试，静默返回成功（200 OK / nil），并严格阻断重复写 Outbox 领域事件及重复广播。
   - **消息撤回链路 (`message_service.RecallMessage`)**：前置检查及 CAS 抢占失败时，若当前消息已是撤回终态，静默返回成功，且严格阻断重复向 WebSocket 广播 `pushRecallNotify`。
8. **PUT 接口幂等性分级审计与自愈（禁止一刀切盲目加 CAS）**：
   - **明确 CAS 适用边界**：CAS 仅适用于“有限状态机流转保护（如拉黑、审批、撤回）”和“多方并发协同写防覆盖（如群资料修改）”。
   - **标量覆写坚持天然幂等**：`PUT /friends/remark`（备注）、`PUT /groups/members/mute`（绝对时间戳）、`PUT /sessions/pin`（置顶）、`PUT /user/info`（个人资料）属于标量覆盖语义（Last-Write-Wins），`SET col = val` 天然具备 100% 幂等性。盲目加旧值 CAS 会导致正常重试被误判为冲突报错或阻断管理员合法业务（如延长禁言）。
   - **群资料乐观锁冲突比对自愈 (`PUT /groups/info`)**：当版本号发生 `CodeConflict` 时，自动取出当前群资料比对请求中非空更新字段。若字段均已完全匹配，判定为自身重试命中，自愈返回成功，兼顾防丢失更新与重试幂等。
9. **POST 申请链路接入第三代 Lua 探针**：
   - `api/proto/apply.proto` 扩展 `client_token`，`ApplyFriend` 与 `ApplyGroup` 全面接入 `pkg/idempotent.Guard`，提供 5s 短租约防并发与 24h 成功状态回显。
   - `SessionService.OpenSession` 接入 `Guard`，结合 `client_token` 与自然业务键实现防并发击穿。

## 结果
- 核心建群接口彻底攻克重试导致重影群聊的缺陷。
- 退群用户重新加群的会话死锁问题彻底根除。
- 用户注册与登录职责清晰解耦，注册接口支持无感幂等重试回放，且坚守了安全凭据接口现场验签与独立生命周期的安全铁律。
- 消息发送链路全面纳入统一状态机幂等体系，攻克了弱网重试导致重复发消息的顽疾。
- 好友拉黑与解拉黑链路升级为严格的数据库原子 CAS，彻底根除了并发重试导致 Outbox 事件重复发派的缺陷。
- 审批单向状态机与消息撤回全面实现 CAS 终态回显自愈，根除了虚假报错与重复副作用。
- 群资料乐观锁实现内容比对自愈，厘清了标量覆盖与 CAS 状态机的本质边界。
- 好友/入群申请与会话打开链路全量接入第三代 Lua 状态机探针。
- 全微服务编译 `go build ./cmd/...` 100% 通过，全量单元测试 100% PASS。

