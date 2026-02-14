# 数据库事务使用指南

本文档说明在 KamaChat 项目中何时需要使用事务，何时不需要，以及背后的原因。

---

## 核心原则

> **事务的本质是保证一组操作的原子性：要么全部成功，要么全部失败回滚。**

---

## ✅ 需要使用事务的场景

### 1. 多表写操作

**场景**：一个业务操作需要修改多张表的数据。

**原因**：如果不使用事务，可能出现"部分成功"的情况，导致数据不一致。

```go
// ❌ 错误示例：没有事务
func CreateGroup() error {
    repos.Group.Create(group)           // 成功
    repos.GroupMember.Create(member)    // 失败！
    repos.Contact.Create(contact)       // 不会执行
    // 结果：群组创建了，但没有群主成员记录 → 数据不一致
}

// ✅ 正确示例：使用事务
func CreateGroup() error {
    return repos.Transaction(func(tx *mysql.Repositories) error {
        tx.Group.Create(group)
        tx.GroupMember.Create(member)
        tx.Contact.Create(contact)
        return nil
        // 任何一步失败，全部回滚
    })
}
```

**本项目中的例子**：
| 业务场景 | 涉及的表 |
|---------|---------|
| 创建群聊 | group_info, group_member, contact, session |
| 解散群聊 | group_info, group_member, contact, session, apply |
| 删除好友 | contact (双向), apply |
| 禁用用户 | user_info, session |
| 通过好友申请 | apply, contact (双向), session (双向) |

---

### 2. 同一表的关联更新

**场景**：对同一张表执行多个有逻辑依赖的更新。

**原因**：保证更新的原子性，避免中间状态被其他请求读取。

```go
// ✅ 需要事务：批量移除群成员后更新成员计数
func RemoveGroupMembers(groupId string, userIds []string) error {
    return repos.Transaction(func(tx *mysql.Repositories) error {
        tx.GroupMember.DeleteByUserUuids(groupId, userIds)    // 删除成员
        tx.Group.DecrementMemberCountBy(groupId, len(userIds)) // 更新计数
        return nil
        // 如果计数更新失败，成员删除也会回滚
    })
}
```

---

### 3. 需要保证业务完整性的操作

**场景**：业务逻辑要求"全有或全无"。

**原因**：即使技术上可以容忍部分失败，但业务上不允许。

```go
// ✅ 通过好友申请：必须同时完成所有操作
func PassFriendApply() error {
    return repos.Transaction(func(tx *mysql.Repositories) error {
        tx.Apply.UpdateStatus(applyId, PASSED)      // 更新申请状态
        tx.Contact.Create(contactA)                  // 创建 A → B 联系人
        tx.Contact.Create(contactB)                  // 创建 B → A 联系人
        tx.Session.Create(sessionA)                  // 创建 A 的会话
        tx.Session.Create(sessionB)                  // 创建 B 的会话
        return nil
        // 如果任何一步失败，申请仍保持待处理状态
    })
}
```

---

## ❌ 不需要使用事务的场景

### 1. 单表单条记录操作

**场景**：只操作一张表的一条或多条记录。

**原因**：单个 SQL 语句本身就是原子的，MySQL 会自动处理。

```go
// ❌ 不需要事务：单表更新
func UpdateUserNickname(userId, nickname string) error {
    return repos.User.UpdateNickname(userId, nickname)
    // 单个 UPDATE 语句，本身就是原子的
}

// ❌ 不需要事务：单表批量更新
func UpdateUserStatusByUuids(uuids []string, status int8) error {
    return repos.User.UpdateUserStatusByUuids(uuids, status)
    // 单个 UPDATE ... WHERE uuid IN (...) 语句
}
```

---

### 2. 纯读取操作

**场景**：只查询数据，不修改任何内容。

**原因**：
- 读操作不会产生"部分修改"的问题
- MySQL 的 MVCC 机制已保证单个 SELECT 的一致性
- 添加事务会增加不必要的开销

```go
// ❌ 不需要事务：JOIN 查询
func FindMembersWithUserInfo(groupId string) ([]Member, error) {
    return db.Table("group_member").
        Joins("LEFT JOIN user_info ON ...").
        Where("group_uuid = ?", groupId).
        Find(&members)
    // 即使涉及多表 JOIN，读操作也不需要事务
}

// ❌ 不需要事务：分页查询
func GetUserList(page, pageSize int) ([]User, error) {
    return repos.User.FindAllPaged(page, pageSize)
}
```

---

### 3. 可容忍部分失败的操作

**场景**：业务上允许某些操作失败，不影响整体。

**原因**：这类操作通常是"尽力而为"的，失败后可以重试或忽略。

```go
// ❌ 不需要事务：清理缓存（失败不影响数据一致性）
func ClearUserCache(userId string) {
    cache.Delete("user_info_" + userId)           // 失败也没关系
    cache.DeleteByPattern("session_list_" + userId + "*")  // 下次访问会重新加载
}

// ❌ 不需要事务：发送通知（失败后可重试）
func NotifyUser(userId, message string) {
    notificationService.Send(userId, message)
    // 即使发送失败，核心业务已完成
}
```

---

### 4. 独立的幂等操作

**场景**：多次执行结果相同的操作。

**原因**：即使失败重试，也不会导致数据异常。

```go
// ❌ 不需要事务：设置用户头像（幂等）
func SetUserAvatar(userId, avatarUrl string) error {
    return repos.User.UpdateAvatar(userId, avatarUrl)
    // 多次执行，结果相同
}
```

---

## 判断流程图

```
是否需要事务？
    │
    ├─ 是否涉及写操作？
    │   │
    │   ├─ 否 → ❌ 不需要事务（纯读取）
    │   │
    │   └─ 是 → 是否涉及多张表？
    │           │
    │           ├─ 否 → 是否是多个有依赖的更新？
    │           │       │
    │           │       ├─ 否 → ❌ 不需要事务
    │           │       │
    │           │       └─ 是 → ✅ 需要事务
    │           │
    │           └─ 是 → ✅ 需要事务
```

---

## 本项目事务使用汇总

### 已使用事务的操作

| 服务 | 函数 | 涉及的表 |
|-----|------|---------|
| GroupService | CreateGroup | group_info, group_member, contact, session |
| GroupService | LeaveGroup | group_member, group_info, contact, apply |
| GroupService | DismissGroup | group_info, group_member, contact, session, apply |
| GroupService | RemoveGroupMembers | group_member, group_info, contact, apply |
| ContactService | DeleteContact | contact, apply |
| ContactService | BlackContact | contact (双向) |
| ApplyService | PassFriendApply | apply, contact (双向), session (双向) |
| ApplyService | PassGroupApply | apply, group_member, group_info, contact, session |
| AdminUserService | BatchUpdateUserStatus (delete) | user_info, session, contact, apply |
| AdminUserService | BatchUpdateUserStatus (disable) | user_info, session |
| AdminGroupService | DeleteGroups | group_info, group_member, contact, session, apply |
| AdminGroupService | SetGroupsStatus (disable) | group_info, session |

### 不需要事务的操作

| 服务 | 函数 | 原因 |
|-----|------|------|
| UserService | UpdateUserInfo | 单表单记录更新 |
| UserService | GetUserInfo | 纯读取 |
| MessageService | GetMessageList | 纯读取 |
| SessionService | GetSessionList | 纯读取 |
| GroupMemberRepo | FindMembersWithUserInfoPaged | 纯读取（虽然是 JOIN） |
| AdminUserService | SetAdmin | 单表批量更新 |

---

## 常见误区

### 误区 1：JOIN 查询需要事务

❌ **错误**：因为 JOIN 涉及多表，所以需要事务。

✅ **正确**：JOIN 是读操作，MySQL 的 MVCC 保证了一致性快照读取。

### 误区 2：所有多步骤操作都需要事务

❌ **错误**：`查询用户 → 更新用户` 需要事务。

✅ **正确**：如果这两步之间没有原子性依赖（即更新失败不需要撤销查询），就不需要事务。

### 误区 3：事务越多越安全

❌ **错误**：为了安全，给所有操作都加事务。

✅ **正确**：不必要的事务会增加锁竞争、降低并发性能。

---

## 总结

| 条件 | 是否需要事务 |
|------|-------------|
| 多表写操作 | ✅ 需要 |
| 同表多个有依赖的写操作 | ✅ 需要 |
| 业务要求全有或全无 | ✅ 需要 |
| 单表单个写操作 | ❌ 不需要 |
| 纯读取操作（包括 JOIN） | ❌ 不需要 |
| 可容忍失败的辅助操作 | ❌ 不需要 |

**记住核心问题**：如果这个操作执行到一半失败了，会不会导致数据处于不一致的状态？如果会，就需要事务。
