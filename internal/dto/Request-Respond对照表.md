# Request - Respond 对照表

## 用户相关

| Request | Respond | 说明 |
|---------|---------|------|
| `LoginRequest` | `LoginRespond` | 用户密码登录 |
| `SmsLoginRequest` | `LoginRespond` | 短信验证码登录 |
| `RegisterRequest` | `RegisterRespond` | 用户注册 |
| `UpdateUserInfoRequest` | `GetUserInfoRespond` | 更新用户信息 |
| `GetUserListPagedRequest` | `GetUserListRespond` | 分页获取用户列表（管理员） |

## 联系人/好友相关

| Request | Respond | 说明 |
|---------|---------|------|
| `ApplyFriendRequest` | - | 申请添加好友（无响应） |
| `BlockContactRequest` | - | 拉黑/取消拉黑联系人（无响应） |
| - | `GetContactInfoRespond` | 获取联系人信息（使用通用结构） |
| - | `GetFriendInfoRespond` | 获取好友信息（使用通用结构） |
| - | `MyUserListRespond` | 我的好友列表 |
| - | `FriendApplyListRespond` | 好友申请列表 |

## 群组相关

| Request | Respond | 说明 |
|---------|---------|------|
| `ApplyGroupRequest` | - | 申请入群（无响应） |
| `CreateGroupRequest` | `GetGroupInfoRespond` | 创建群聊 |
| `UpdateGroupInfoRequest` | `GetGroupInfoRespond` | 更新群聊信息 |
| `DismissGroupRequest` | - | 解散群聊（无响应） |
| `LeaveGroupRequest` | - | 退出群聊（无响应） |
| `RemoveGroupMembersRequest` | - | 移除群成员（无响应） |
| `SetGroupsStatusRequest` | - | 批量设置群组状态（无响应） |
| `PagedRequest` | `GetGroupListRespond` | 分页获取群组列表 |
| `PagedRequest` | `GetGroupListWrapper` | 群组列表包装（含分页） |
| - | `GetGroupDetailRespond` | 获取群组详情 |
| - | `PublicGroupInfoRespond` | 公开群组信息 |
| - | `MyGroupListRespond` | 我的群组列表 |
| - | `GetGroupMemberListRespond` | 群成员列表 |

## 申请相关

| Request | Respond | 说明 |
|---------|---------|------|
| `GetGroupApplyListRequest` | `GroupApplyListRespond` | 获取入群申请列表 |
| `PassFriendApplyRequest` | - | 通过/拒绝好友申请（无响应） |
| `PassGroupApplyRequest` | - | 通过/拒绝入群申请（无响应） |
| `RejectFriendApplyRequest` | - | 拒绝好友申请（无响应） |
| `RejectGroupApplyRequest` | - | 拒绝入群申请（无响应） |

## 会话相关

| Request | Respond | 说明 |
|---------|---------|------|
| `CreateSessionRequest` | `string` | 创建会话（返回 session_id） |
| `OpenSessionRequest` | `string` | 打开会话（返回 session_id） |
| `CheckSessionAllowedRequest` | `bool` | 检查会话权限（返回是否允许） |
| `BatchDeleteRequest` | - | 删除会话（无响应） |
| - | `UserSessionListRespond` | 用户会话列表 |
| - | `GroupSessionListRespond` | 群组会话列表 |

## 消息相关

| Request | Respond | 说明 |
|---------|---------|------|
| `ChatMessageRequest` | - | 聊天消息（WebSocket，无响应） |
| `GetMessageListRequest` | `GetMessageListRespond` | 获取聊天记录 |
| - | `GetGroupMessageListRespond` | 获取群聊消息 |
| - | `AVMessageRespond` | 音视频消息 |

## 管理员相关

| Request | Respond | 说明 |
|---------|---------|------|
| `BatchUpdateUserStatusRequest` | - | 批量更新用户状态（无响应） |
| `SetAdminRequest` | - | 设置管理员（无响应） |
| `BatchDeleteRequest` | - | 批量删除群组（无响应） |

## 其他

| Request | Respond | 说明 |
|---------|---------|------|
| `SendSmsCodeRequest` | - | 发送短信验证码（无响应） |
| `RefreshTokenRequest` | - | 刷新 Token（无响应） |

---

## 说明

1. **无响应的操作**：部分操作（如删除、更新状态、拉黑等）只返回成功/失败状态，不返回具体数据
2. **通用结构**：部分查询操作使用通用的响应结构（如 `GetUserInfoRespond`、`GetGroupInfoRespond`）
3. **WebSocket**：`ChatMessageRequest` 是 WebSocket 消息，不使用传统的 Request-Respond 模式
4. **基础复用**：`PagedRequest` 可用于多个分页接口
5. **批量操作**：所有批量操作统一使用 `UuidList` 字段，支持单个或批量操作

---

## 优化前后对比

### 优化前
- 38 个 request 文件
- 多个重复的删除请求结构
- 多个重复的 ID 请求结构
- 字段名不一致（`pageSize` vs `page_size`）

### 优化后
- 30 个 request 文件（减少 8 个）
- 统一的 `BatchDeleteRequest`（删除联系人/群组/会话）
- 统一的 `PagedRequest`（分页基础结构）
- 字段名统一规范
- 删除未使用的结构体
