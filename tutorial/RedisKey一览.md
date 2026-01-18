# RedisKey 一览（KamaChat）

> 本文根据当前代码仓库中对 `CacheService/AsyncCacheService` 的调用点整理（截至 2026-01-10）。
> 
> 说明：项目里同时存在 `:` 与 `_` 两种分隔风格（例如 `user_token:<uid>`、`user_info_<uid>`）。本文按现状记录。

## 全局 TTL 常量

- `constants.REDIS_TIMEOUT`：用于部分列表/会话缓存，单位“分钟”。当前值为 `1`。
- `constants.REFRESH_TOKEN_EXPIRY_HOURS`：用于 RefreshToken 的单点登录互踢缓存，单位“小时”。当前值为 `168`（7 天）。

## Key 列表

下面的「类型」指 Redis 数据结构。

### 短信验证码

- Key：`auth_code_<telephone>`
  - 类型：String
  - 值：6 位数字验证码（纯字符串）
  - TTL：`1m`
  - 写入：发送验证码时写入（用于频率限制 + 取码校验）
  - 读取：验证码登录/注册时校验
  - 删除：校验成功后主动 `DEL`

### 单点登录 / RefreshToken 互踢

- Key：`user_token:<userId>`
  - 类型：String
  - 值：RefreshToken 的 `tokenID`
  - TTL：`constants.REFRESH_TOKEN_EXPIRY_HOURS` 小时（默认 7 天）
  - 写入：登录/验证码登录成功后写入
  - 读取：鉴权中校验 `tokenID` 是否与 Redis 中一致

### 用户信息缓存

- Key：`user_info_<userId>`
  - 类型：String
  - 值：`respond.GetUserInfoRespond` 的 JSON
  - TTL：`1h`
  - 读取：获取用户信息 / 校验用户状态（部分流程优先读缓存）
  - 写入：
    - 获取用户信息时异步回写
    - 获取好友详情时回写（同 key）
  - 失效/清理：
    - 修改用户资料后删除该 key
    - 管理员批量禁用/删除/设管理员等场景会批量删除相关 key

### 群信息缓存

- Key：`group_info_<groupId>`
  - 类型：String
  - 值：`respond.GetGroupInfoRespond` 的 JSON
  - TTL：
    - 群模块：`24h`
    - 联系人模块：`1h`
  - 读取：获取群信息/加群方式/会话前置校验等
  - 写入：获取群信息、检查加群方式、获取群详情等会回写
  - 失效/清理：进群/退群/解散/更新群信息/移除成员/批量删除群等会删除该 key

> 注意：同一个 `group_info_<groupId>` 由不同模块以不同 TTL 写入，最终 TTL 取决于“最近一次写入者”。

### 联系人关系（好友列表）

- Key：`contact_relation:user:<userId>`
  - 类型：Set
  - 成员：好友用户 ID（例如 `Uxxxxxxxxxxx`）
  - TTL：未设置（持久）
  - 写入：查询好友列表未命中时，从 DB 回填 `SADD`
  - 更新：删除联系人时会 `SREM`（仅移除当前用户视角下的成员）
  - 失效/清理：加好友通过、拉黑/取消拉黑、批量删用户等会删除/按模式删除

### 联系人关系（我加入的群）

- Key：`contact_relation:group:<userId>`
  - 类型：Set
  - 成员：群 ID（例如 `Gxxxxxxxxxxx`）
  - TTL：未设置（持久）
  - 写入：查询“我加入的群（不含自建）”未命中时，从 DB 回填 `SADD`
  - 更新：退群时会 `SREM`
  - 失效/清理：进群/退群/解散群/移除成员/批量删群等会删除/按模式删除

### 我创建的群列表

- Key：`contact_mygroup_list_<userId>`
  - 类型：String
  - 值：`[]respond.LoadMyGroupRespond` 的 JSON
  - TTL：`30m`
  - 写入：查询“我创建的群”列表时回写
  - 失效/清理：创建群/解散群/更新群信息/批量删群等会删除（通常用 `DeleteByPattern`）

### 单聊消息列表缓存

- Key：`message_list_<userIdSmall>_<userIdLarge>`
  - 类型：String
  - 值：`[]respond.GetMessageListRespond` 的 JSON
  - TTL：`constants.REDIS_TIMEOUT` 分钟（默认 1 分钟）
  - 写入：获取聊天记录时异步回写；发送消息时也会“追加并回写”

> 注意：有的发送消息逻辑构造 key 时未做 `userId` 排序（会导致同一对用户出现两份 key）。建议统一按 `min/max` 规则生成。

### 群聊消息列表缓存

- Key：`group_messagelist_<groupId>`
  - 类型：String
  - 值：`[]respond.GetGroupMessageListRespond` 的 JSON
  - TTL：`constants.REDIS_TIMEOUT` 分钟（默认 1 分钟）
  - 写入：获取群消息记录时异步回写；发送群消息时也会“追加并回写”

### 打开会话缓存（单条会话）

- Key：`session_<sendId>_<receiveId>`
  - 类型：String
  - 值：`model.Session` 的 JSON
  - TTL：`constants.REDIS_TIMEOUT` 分钟（默认 1 分钟）
  - 写入：打开会话（OpenSession）查库后回写
  - 备注：当前更多是“短期热点缓存”，主要的列表缓存见下方两个 key。

### 私聊会话列表缓存

- Key：`direct_session_list_<ownerId>`
  - 类型：String
  - 值：`[]respond.UserSessionListRespond` 的 JSON
  - TTL：`constants.REDIS_TIMEOUT` 分钟（默认 1 分钟）
  - 写入：获取私聊会话列表时回写
  - 失效/清理：创建会话/删除会话/删除联系人/拉黑/用户禁用或删除等会删除（常用 `DeleteByPattern`）

### 群聊会话列表缓存

- Key：`group_session_list_<ownerId>`
  - 类型：String
  - 值：`[]respond.GroupSessionListRespond` 的 JSON
  - TTL：`constants.REDIS_TIMEOUT` 分钟（默认 1 分钟）
  - 写入：获取群聊会话列表时回写
  - 失效/清理：创建会话/删除会话/进群退群/解散群/用户禁用或删除等会删除（常用 `DeleteByPattern`）

### 群成员列表缓存

- Key：`group_memberlist_<groupId>`
  - 类型：String
  - 值：`[]respond.GetGroupMemberListRespond` 的 JSON
  - TTL：`24h`
  - 写入：获取群成员列表时回写
  - 失效/清理：进群/退群/解散群/移除成员/批量删群等会删除

## 仅在清理逻辑中出现的 Key/Pattern（可能是历史遗留）

- Pattern：`session_list_<userId>*`
  - 现状：在会话缓存清理函数里会按该模式删除，但当前仓库未检索到对应的写入点。
  - 建议：若确认已废弃，可以统一移除清理逻辑；若仍需要，请补齐写入端并在此文档补充结构与 TTL。

## 维护建议（新增/调整 Key 时）

- 统一命名风格（建议只选一种分隔符风格，例如全部 `:` 分层）。
- 明确：数据结构（String/Set/Hash/List）、值的 JSON DTO 类型、TTL、失效策略（哪些写操作需要删哪些 key）。
- 避免 `DeleteByPattern` 的大范围扫描成为热点路径；优先精确删除 key。
