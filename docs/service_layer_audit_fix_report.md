# Service 层逻辑审计与修复报告

> 审计时间：2026-02-07
> 审计范围：`internal/service/` 下所有业务逻辑文件
> 编译验证：`go build ./...` 通过

---

## 修复总览

| 优先级 | 已修复数 | 说明 |
|--------|---------|------|
| P0（安全/数据损坏） | 2 | 登录禁用绕过、消息越权访问 |
| P1（功能缺陷） | 6 | 好友删除单向、channel panic、竞态条件、状态校验缺失、群主退群 |
| P2（逻辑不严谨） | 4 | 自我添加好友、拒绝/拉黑状态重复处理、会话权限、缓存一致性 |
| P3（优化/清理） | 5 | 效率优化、字段清空、缓存清理、死代码清理 |
| **合计** | **17** | |

---

## P0：安全/数据完整性问题

### #1 Login / SmsLogin 未检查用户禁用状态
**文件**: `internal/service/user/service.go:68-132, 134-206`
**问题**: 被管理员禁用的用户仍可正常登录并获取 Token。
**修复**: 在密码/验证码校验通过后、生成 Token 前，添加 `user.Status == user_status_enum.DISABLE` 检查，返回 `CodeForbidden` 错误。

### #6 GetMessageList 缺少权限校验 + 未返回 total
**文件**: `internal/service/message/service.go`, `internal/handler/message_handler.go`
**问题**:
1. 任意用户可通过传入他人 ID 查看私聊消息（IDOR 漏洞）。
2. 分页接口未返回 `total`，前端无法正确渲染分页控件。

**修复**:
1. 添加好友关系/群成员身份校验。
2. Repository 层新增 `Count` 查询，Service 层返回 `([]MessageRespond, int64, error)` 三元组。
3. Handler 层适配新签名。

---

## P1：功能缺陷

### #3 会话列表分页 total 不准确
**文件**: `internal/service/session/service.go`, `internal/dao/mysql/interfaces.go`, `internal/dao/mysql/session/session_repository.go`
**问题**: `GetUserSessionList` 和 `GetGroupSessionList` 使用通用的 `FindBySendIdPaged` 查询后在内存中按 `ReceiveId` 前缀过滤，导致 `total` 是全部会话数而非过滤后的数量。
**修复**: 新增 `FindBySendIdAndTypePaged(sendId, receiveIdPrefix, page, pageSize)` 方法，在 SQL 层通过 `receive_id LIKE ?` 过滤，Count 和分页查询使用同一条件。

### #7 DeleteContact 仅单向删除
**文件**: `internal/service/contact/service.go`
**问题**: 删除好友时只删除了 `Me -> Friend` 方向的关系，`Friend -> Me` 仍然保留，导致对方仍可看到已删除的好友。
**修复**: 在事务中同时执行双向删除 (`SoftDelete(userId, contactId)` + `SoftDelete(contactId, userId)`)，同时双向删除申请记录和缓存。

### #8 kafka_broker channel 双重 close
**文件**: `internal/service/chat/kafka_broker.go`
**问题**: `MsgConsumer.Start()` 的 `defer` 和 `Close()` 方法都会关闭 `Login`/`Logout` channel，存在 panic 风险。
**修复**: 在 `MsgConsumer` 结构体中添加 `closeOnce sync.Once` 字段，将 channel 关闭操作封装在 `sync.Once.Do()` 中。

### #9 ClientLogout 竞态条件
**文件**: `internal/service/chat/ws_gateway.go`
**问题**: `ClientLogout` 先关闭 WebSocket 连接再关闭 channel，可能导致正在写入 channel 的 goroutine 与关闭操作竞态。
**修复**: 调整操作顺序为：`UnregisterClient` → `close(SendBack)` → `close(SendTo)` → `Conn.Close()`。每个 channel 关闭操作使用 `defer recover()` 防护双重关闭。

### #11 PassFriendApply / PassGroupApply 缺少申请状态校验
**文件**: `internal/service/apply/service.go`
**问题**: 未校验申请是否处于 `PENDING` 状态，已同意/拒绝/拉黑的申请可被重复处理。
**修复**: 在处理前添加 `apply.Status != contact_apply_status_enum.PENDING` 检查。

### #14 LeaveGroup 群主可直接退群
**文件**: `internal/service/group/service.go:313-378`
**问题**: 群主退群后群组变成无主状态，无人可管理。
**修复**: 获取成员角色后检查 `member.Role == 3`（群主），阻止退群并提示"请先转让群主或解散群聊"。

---

## P2：逻辑不严谨

### #12 RefuseFriendApply / BlackFriendApply 等缺少状态校验
**文件**: `internal/service/apply/service.go`
**问题**: `RefuseFriendApply`、`RefuseGroupApply`、`BlackFriendApply`、`BlackGroupApply` 四个方法均未校验申请状态，允许对非 PENDING 状态的申请进行操作。
**修复**: 统一添加 `apply.Status != PENDING` 前置检查。

### #13 ApplyFriend 允许添加自己
**文件**: `internal/service/apply/service.go`
**问题**: 用户可以向自己发送好友申请。
**修复**: 添加 `userId == req.FriendId` 前置检查。

### #15 CheckOpenSessionAllowed 不支持群聊会话
**文件**: `internal/service/session/service.go:211-262`
**问题**: 原实现仅处理用户间的联系关系，当 `receiveId` 为群组 ID（`G` 前缀）时会查询失败。
**修复**: 根据 `receiveId[0]` 前缀分别走用户（检查黑名单）和群组（检查成员身份）逻辑。同时抽取 `checkTargetStatusWithCache` 方法统一处理用户/群组的禁用状态检查。

### #16 sendToUser 缓存 Key 顺序不一致
**文件**: `internal/service/chat/kafka_broker.go`
**问题**: `sendToUser` 构造缓存 Key 为 `"message_list_" + sendId + "_" + receiveId`，而 `GetMessageList` 中对两个 ID 排序后构造 Key，导致同一对话的缓存 Key 不一致，缓存失效时机错误。
**修复**: 在 `sendToUser` 中对 `sendId` 和 `receiveId` 排序后再拼接 Key，保持与 `GetMessageList` 一致。

---

## P3：优化与清理

### #2 移除死代码
**文件**: `internal/service/user/service.go`, `internal/service/chat/kafka_broker.go`
**修复**:
1. 移除未使用的 `checkUserIsAdminOrNot` 方法，注册处直接赋值 `newUser.IsAdmin = 0`。
2. 移除未使用的 `updateRedisGroup` 方法。

### #4 DeleteSession 效率优化
**文件**: `internal/service/session/service.go:455-486`, `internal/dao/mysql/interfaces.go`, `internal/dao/mysql/session/session_repository.go`
**问题**: 原实现通过 `FindBySendId(ownerId)` 加载用户的所有会话后遍历匹配，时间复杂度 O(n)。
**修复**: 新增 `SessionRepository.FindByUuid(uuid)` 方法，直接按 UUID 主键查询会话后验证 `SendId == ownerId`，时间复杂度 O(1)。

### #10 UpdateUserInfo 无法清空字段
**文件**: `internal/service/user/service.go:283-327`, `internal/dto/request/user/update_user_info_request.go`
**问题**: DTO 字段为 `string` 类型，`if field != ""` 导致无法将已有值清空为空字符串。
**修复**: 将 DTO 字段改为 `*string` 指针类型。`nil` = 未传（不更新），`""` = 清空。Service 层判断改为 `if field != nil`。

### #17 GetGroupList 缓存优化
**说明**: `LoadMyGroup` 基于 `FindByOwnerIdPaged` 分页查询，添加按页缓存会增加缓存失效维护的复杂度，但性能收益有限。按 YAGNI 原则暂不实施，标记为已评估跳过。

### Admin enable 操作缓存清理
**文件**: `internal/service/admin/user/service.go:65-79`
**问题**: `BatchUpdateUserStatus` 的 `enable` 分支更新数据库状态后未清理 `user_info_` 缓存，导致已启用的用户在缓存过期前仍被视为禁用状态。
**修复**: 添加异步缓存清理逻辑，与 `disable` 分支保持一致。

---

## 修改文件清单

| 文件路径 | 修改项 |
|---------|--------|
| `internal/service/user/service.go` | #1 登录禁用检查, #2 死代码移除, #10 清空字段 |
| `internal/service/message/service.go` | #6 权限校验 + total |
| `internal/handler/message_handler.go` | #6 适配新签名 |
| `internal/service/session/service.go` | #3 分页修复, #4 效率优化, #15 群组兼容 |
| `internal/service/contact/service.go` | #7 双向删除 |
| `internal/service/chat/kafka_broker.go` | #8 channel 安全, #16 缓存 Key, #2 死代码 |
| `internal/service/chat/ws_gateway.go` | #9 竞态修复 |
| `internal/service/apply/service.go` | #11 #12 #13 状态校验 |
| `internal/service/group/service.go` | #14 群主退群限制 |
| `internal/service/admin/user/service.go` | enable 缓存清理 |
| `internal/dao/mysql/interfaces.go` | #3 新接口, #4 FindByUuid, #6 签名变更 |
| `internal/dao/mysql/session/session_repository.go` | #3 #4 新实现 |
| `internal/dao/mysql/message/message_repository.go` | #6 Count 查询 |
| `internal/dto/request/user/update_user_info_request.go` | #10 指针类型 |
