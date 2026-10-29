# 0008. 三层并发与 MySQL 持久化幂等表架构

- **状态**: Accepted
- **日期**: 2026-10-29

## 背景
在高并发与重试场景下，接口面临“用户手抖短时连击（秒级重复）”、“多用户并发修改同一实体导致竞态超员/死锁（并发冲突）”以及“网络超时重试导致重复生成流水/多建群（重试穿透）”三重交织挑战。此前项目依赖单一内存 Redis Lua 状态机，缺少对分布式共享实体的显式互斥锁，且内存数据在服务或缓存重启后存在重试穿透风险。

## 决定
引入源自 `D:\testaba` 的模式，构建“中间件防抖 + 实体分布式锁 + MySQL 持久化幂等表”三层协同防御体系：

1. **第一层：Kratos 中间件全局写防抖（RequestDebounce）**：
   - 基于 Redis `SETNX debounce:{userId}:{operation}:{sha256(payload)} 1s`；
   - 覆盖所有写操作（POST / PUT / DELETE），秒级连击直接返回 409 `CodeConflict`（请求过于频繁），不进入业务事务；只读查询（GET）自动豁免。

2. **第二层：热点共享实体分布式锁（DistributedLock）**：
   - 采用 Redis `SETNX` + 随机 Token + Lua 脚本原子释放；
   - 针对跨请求竞争的特定共享实体进行串行化保护：
     - 双向好友关系：`lock:friendship:min(u1,u2):max(u1,u2)`，杜绝 A/B 互相并发申请/通过导致的死锁与双向脏数据；
     - 群成员容量变动：`lock:group:member:{groupId}`，杜绝多管理员并发拉人导致群成员超出容量上限；
     - 群主身份转移：`lock:group:owner:{groupId}`，杜绝并发转让导致群主状态撕裂。

3. **第三层：MySQL 持久化幂等表（IdempotencyTable）**：
   - 废弃内存临时回放方案，全量采用 MySQL `idempotency_record` 物理表，利用唯一索引 `uk_idempotency(user_id, operation, idempotency_key)` 与业务事务（Unit of Work）强绑定；
   - **强制纳入范围**：`SendMessage`（发消息）、`CreateGroup`（建群）、`Register`（用户注册）、`ApplyFriend`/`ApplyGroup`（申请单创建）、`PassFriendApply`/`PassGroupApply`（审批建链）；
   - **自然幂等豁免**：PUT 资料/备注/禁言覆盖更新、DELETE 删除好友/解散群走自然幂等，不冗余插入幂等表；
   - **版本号 CAS 豁免**：`BlackFriend` / `UnblackFriend` 继续沿用乐观锁 Version CAS 防 ABA；
   - **反幂等防重放**：`Login` 与 `RefreshToken` 严格禁止幂等回放，每次强制重新签发新 Token。

4. **幂等键提取规范（IdempotencyKey）**：
   - 双通道优先匹配：优先提取请求头 `X-Idempotency-Key`（或 `Idempotency-Key`），次选结构体内 `ClientMsgId` / `ClientToken`；
   - 高价值写接口若双通道均缺失，直接返回 400 `InvalidParam` 快速失败，强制客户端提供唯一幂等凭证。

## 影响与后果
- 全面消除用户连击、并发抢占以及网络重试引发的重演与脏数据风险；
- 发消息接口使用 MySQL 幂等表增加了单条消息的 DB 事务开销，但换取了跨宕机恢复的绝对持久化与强一致性；
- 形成清晰的接口准入分类矩阵，后续新接口开发可严格对照象限套用。
