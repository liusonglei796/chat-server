# Redis 键值文档
本文档列出了 KamaChat 应用中所有 Redis 键值，按模块分类。

---

## 1. 认证与用户
| 键值模式 | 类型 | 说明 | 过期时间 | 使用位置 |
|----------|------|------|----------|----------|
| `auth_code_<telephone>` | String | 存储登录/注册用的短信验证码 | 约 1 分钟 | sms/auth_code_service, user/service |
| `user_token:<uuid>` | String | 存储刷新令牌 ID，用于单设备登录/踢出逻辑 | REFRESH_TOKEN_EXPIRY_HOURS | user/service, auth/service |
| `user_info_<uuid>` | String | 缓存 UserInfoRespond JSON（完整用户资料） | 约 24 小时 | user/service, contact/service, session/service |

---

## 2. 联系人关系
| 键值模式 | 类型 | 说明 | 过期时间 | 使用位置 |
|----------|------|------|----------|----------|
| `contact_relation:user:<userId>` | Set | 存储与 userId 为好友关系的用户 UUID 集合 | 永久有效（同步更新） | contact/service, apply/service |
| `contact_relation:group:<userId>` | Set | 存储 userId 已加入的群组 UUID 集合 | 永久有效（同步更新） | contact/service, group/service, apply/service |

---

## 3. 群组信息
| 键值模式 | 类型 | 说明 | 过期时间 | 使用位置 |
|----------|------|------|----------|----------|
| `group_info_<groupId>` | String | 缓存 GetGroupInfoRespond JSON（群组详情） | 约 24 小时 | group/service, contact/service, session/service, apply/service |
| `group_memberlist_<groupId>` | String | 缓存 `[]GetGroupMemberListRespond` JSON（成员列表） | 约 24 小时 | group/service, apply/service |


---

## 4. 会话（聊天列表）
| 键值模式 | 类型 | 说明 | 过期时间 | 使用位置 |
|----------|------|------|----------|----------|
| `direct_session_list_<userId>` | String | 缓存 `[]UserSessionListRespond`（私聊会话列表） | REDIS_TIMEOUT（环境变量） | session/service, contact/service |
| `group_session_list_<userId>` | String | 缓存 `[]GroupSessionListRespond`（群聊会话列表） | REDIS_TIMEOUT（环境变量） | session/service, group/service, apply/service |
| `session_<sendId>_<receiveId>` | String | 缓存 model.Session JSON（会话元数据） | REDIS_TIMEOUT（环境变量） | session/service |
| `session_list_<userId>` | String | 遗留/保留字段？在清理逻辑中出现，但未找到活跃的设置操作 | - | session/service |

---

## 5. 消息（聊天记录）
| 键值模式 | 类型 | 说明 | 过期时间 | 使用位置 |
|----------|------|------|----------|----------|
| `message_list_<user1>_<user2>` | String | 缓存 `[]GetMessageListRespond`（私聊历史记录），ID 按字典序排序 | REDIS_TIMEOUT（环境变量） | message/service, chat/server（channel/kafka） |
| `group_messagelist_<groupId>` | String | 缓存 `[]GetGroupMessageListRespond`（群聊历史记录） | REDIS_TIMEOUT（环境变量） | message/service, chat/server（channel/kafka） |

---

## 键值管理说明
**清理策略**：大多数键值采用"Cache-Aside"或"Write-Delete"策略管理。当数据变更（数据库更新/创建）时，相关的缓存键通常通过 SubmitTask 异步删除（或更新）。

**模式匹配**：部分删除操作使用 DeleteByPattern（例如 `group_session_list_<userId>*`）以确保清除所有相关变体，尽管主键通常不带后缀。