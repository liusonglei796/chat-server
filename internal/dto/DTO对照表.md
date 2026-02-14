# DTO 对照表

## Request 结构体（请求）

### 基础结构
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `paged_request.go` | `PagedRequest` | 基础分页请求（Page, PageSize） |
| `batch_delete_request.go` | `BatchDeleteRequest` | 批量删除请求（UuidList） |

### 用户相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `login_request.go` | `LoginRequest` | 用户密码登录 |
| `sms_login_request.go` | `SmsLoginRequest` | 短信验证码登录 |
| `register_request.go` | `RegisterRequest` | 用户注册 |
| `update_user_info_request.go` | `UpdateUserInfoRequest` | 更新用户信息 |
| `get_user_list_paged_request.go` | `GetUserListPagedRequest` | 分页获取用户列表（管理员，含筛选） |

### 联系人/好友相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `apply_friend_request.go` | `ApplyFriendRequest` | 申请添加好友 |
| `block_contact_request.go` | `BlockContactRequest` | 拉黑/取消拉黑联系人 |
| `update_remark_request.go` | `UpdateRemarkRequest` | 更新好友备注请求 |

### 群组相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `apply_group_request.go` | `ApplyGroupRequest` | 申请入群 |
| `create_group_request.go` | `CreateGroupRequest` | 创建群聊 |
| `update_group_info_request.go` | `UpdateGroupInfoRequest` | 更新群聊信息 |
| `dismiss_group_request.go` | `DismissGroupRequest` | 解散群聊 |
| `leave_group_request.go` | `LeaveGroupRequest` | 退出群聊 |
| `remove_group_members_request.go` | `RemoveGroupMembersRequest` | 移除群成员（批量） |
| `set_groups_status_request.go` | `SetGroupsStatusRequest` | 批量设置群组状态 |
| `mute_member_request.go` | `MuteMemberRequest` | 群成员禁言请求 |

### 申请相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `get_group_apply_list_request.go` | `GetGroupApplyListRequest` | 获取入群申请列表 |
| `pass_friend_apply_request.go` | `PassFriendApplyRequest` | 通过/拒绝好友申请 |
| `pass_group_apply_request.go` | `PassGroupApplyRequest` | 通过/拒绝入群申请 |
| `reject_friend_apply_request.go` | `RejectFriendApplyRequest` | 拒绝好友申请 |
| `reject_group_apply_request.go` | `RejectGroupApplyRequest` | 拒绝入群申请 |

### 会话相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `create_session_request.go` | `CreateSessionRequest` | 创建会话 |
| `open_session_request.go` | `OpenSessionRequest` | 打开会话 |
| `check_session_allowed_request.go` | `CheckSessionAllowedRequest` | 检查会话权限 |
| `pin_session_request.go` | `PinSessionRequest` | 会话置顶请求 |

### 消息相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `chat_message_request.go` | `ChatMessageRequest` | 聊天消息（WebSocket） |
| `chat_message_request.go` | `AVData` | 音视频消息数据 |
| `get_message_list_request.go` | `GetMessageListRequest` | 获取聊天记录 |
| `recall_message_request.go` | `RecallMessageRequest` | 消息撤回请求 |

### 管理员相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `batch_update_user_status_request.go` | `BatchUpdateUserStatusRequest` | 批量更新用户状态 |
| `set_admin_request.go` | `SetAdminRequest` | 设置管理员（批量） |

### 其他
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `send_sms_code_request.go` | `SendSmsCodeRequest` | 发送短信验证码 |
| `refresh_token_request.go` | `RefreshTokenRequest` | 刷新 Token |

---

## Respond 结构体（响应）

### 用户相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `get_userinfo_respond.go` | `GetUserInfoRespond` | 获取用户信息 |
| `get_userlist_respond.go` | `GetUserListRespond` | 用户列表项 |
| `public_userinfo_respond.go` | `PublicUserInfoRespond` | 公开用户信息 |
| `login_respond.go` | `LoginRespond` | 登录响应 |
| `register_respond.go` | `RegisterRespond` | 注册响应 |

### 联系人/好友相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `get_contactinfo_respond.go` | `GetContactInfoRespond` | 获取联系人信息 |
| `get_friend_info_respond.go` | `GetFriendInfoRespond` | 获取好友信息 |
| `my_userlist_respond.go` | `MyUserListRespond` | 我的好友列表 |
| `friend_apply_list_respond.go` | `FriendApplyListRespond` | 好友申请列表 |

### 群组相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `get_groupinfo_respond.go` | `GetGroupInfoRespond` | 获取群组信息 |
| `get_group_detail_respond.go` | `GetGroupDetailRespond` | 获取群组详情 |
| `public_groupinfo_respond.go` | `PublicGroupInfoRespond` | 公开群组信息 |
| `get_grouplist_respond.go` | `GetGroupListRespond` | 群组列表项 |
| `get_grouplist_respond.go` | `GetGroupListWrapper` | 群组列表包装（含分页） |
| `my_grouplist_respond.go` | `MyGroupListRespond` | 我的群组列表 |
| `get_groupmember_list_respond.go` | `GetGroupMemberListRespond` | 群成员列表 |
| `group_apply_list_respond.go` | `GroupApplyListRespond` | 入群申请列表 |

### 会话相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `user_sessionlist_respond.go` | `UserSessionListRespond` | 用户会话列表 |
| `group_sessionlist_respond.go` | `GroupSessionListRespond` | 群组会话列表 |

### 消息相关
| 文件 | 结构体 | 说明 |
|------|--------|------|
| `get_message_list_respond.go` | `GetMessageListRespond` | 聊天记录 |
| `get_group_messagelist_respond.go` | `GetGroupMessageListRespond` | 群聊消息 |
| `av_message_respond.go` | `AVMessageRespond` | 音视频消息 |

---

## 统计

- **Request 结构体**: 30 个文件，32 个结构体（部分文件包含多个）
- **Respond 结构体**: 22 个文件，约 25 个结构体
- **已删除的重复/未使用结构体**: 15 个

## 优化成果

1. ✅ 创建基础结构（`PagedRequest`, `BatchDeleteRequest`）
2. ✅ 合并重复的删除请求（统一使用 `BatchDeleteRequest`）
3. ✅ 优化分页请求（使用组合方式）
4. ✅ 删除未使用的结构体（`IDRequest`, `UUIDRequest`）
5. ✅ 重命名结构体以更清晰表达用途
6. ✅ 修复 IDOR 安全问题（从 JWT 获取用户 ID）
