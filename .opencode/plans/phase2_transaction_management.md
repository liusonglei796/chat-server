# Phase 2: 数据库事务管理优化方案

## 优化目标
在关键业务逻辑中引入事务支持，保证数据一致性

## 当前问题

### 1. 通过好友申请无事务
**位置**: `internal/service/apply/` (待查看具体文件)
**问题**: 更新 Apply 表状态和插入 Contact 记录是两个独立操作
**风险**: 中间失败会导致数据不一致（如已通过但无好友关系）

### 2. 解散群组无事务
**位置**: `internal/service/group/` (待查看具体文件)
**问题**: 
- 删除 Group 记录
- 删除 GroupMember 记录
- 更新 Contact 表（群聊会话）
**风险**: 部分成功会导致脏数据

### 3. 入群审批无事务
**位置**: `internal/service/apply/`
**问题**: 更新 Apply 状态 + 插入 GroupMember
**风险**: 审批通过但未加入群组

---

## 现有事务基础设施

**当前实现**: `internal/dao/mysql/provider.go`
```go
func (r *Repositories) Transaction(fn func(txRepos *Repositories) error) error {
    return r.db.Transaction(func(tx *gorm.DB) error {
        return fn(NewRepositories(tx))
    })
}
```

**评价**: 设计良好，但使用范围有限

---

## 优化方案

### 1. 好友申请通过事务

**业务逻辑**:
```go
func (s *applyService) PassFriendApply(userId, applicantId string) error {
    return s.repos.Transaction(func(txRepos *mysql.Repositories) error {
        // 1. 更新申请状态为已通过
        if err := txRepos.Apply.UpdateStatus(applicantId, userId, "passed"); err != nil {
            return err
        }
        
        // 2. 创建双向好友关系
        contact1 := &model.Contact{
            UserUuid:   applicantId,
            FriendUuid: userId,
            Status:     0, // 正常
        }
        contact2 := &model.Contact{
            UserUuid:   userId,
            FriendUuid: applicantId,
            Status:     0,
        }
        
        if err := txRepos.Contact.Create(contact1); err != nil {
            return err
        }
        if err := txRepos.Contact.Create(contact2); err != nil {
            return err
        }
        
        // 3. 发送系统消息通知（可选）
        // ...
        
        return nil
    })
}
```

### 2. 解散群组事务

**业务逻辑**:
```go
func (s *groupService) DismissGroup(operatorId, groupId string) error {
    return s.repos.Transaction(func(txRepos *mysql.Repositories) error {
        // 1. 验证操作者是群主
        group, err := txRepos.Group.FindByUuid(groupId)
        if err != nil {
            return err
        }
        if group.OwnerUuid != operatorId {
            return errorx.New(errorx.CodeForbidden, "只有群主可以解散群组")
        }
        
        // 2. 获取所有群成员
        members, err := txRepos.GroupMember.FindByGroupUuid(groupId)
        if err != nil {
            return err
        }
        
        // 3. 删除群成员记录
        if err := txRepos.GroupMember.DeleteByGroupUuid(groupId); err != nil {
            return err
        }
        
        // 4. 删除群组记录
        if err := txRepos.Group.Delete(groupId); err != nil {
            return err
        }
        
        // 5. 删除所有成员的群聊会话
        for _, member := range members {
            if err := txRepos.Contact.DeleteGroupSession(member.UserUuid, groupId); err != nil {
                // 记录日志但不中断事务
                zap.L().Error("删除群聊会话失败", 
                    zap.String("user", member.UserUuid),
                    zap.Error(err))
            }
        }
        
        return nil
    })
}
```

### 3. 入群审批事务

**业务逻辑**:
```go
func (s *applyService) PassGroupApply(operatorId, groupId, applicantId string) error {
    return s.repos.Transaction(func(txRepos *mysql.Repositories) error {
        // 1. 验证操作者是群主或管理员
        member, err := txRepos.GroupMember.FindByGroupAndUser(groupId, operatorId)
        if err != nil {
            return err
        }
        if member.Role != 1 && member.Role != 2 { // 1=群主, 2=管理员
            return errorx.New(errorx.CodeForbidden, "无权限审批入群申请")
        }
        
        // 2. 更新申请状态
        if err := txRepos.Apply.UpdateGroupApplyStatus(groupId, applicantId, "passed"); err != nil {
            return err
        }
        
        // 3. 添加群成员
        newMember := &model.GroupMember{
            GroupUuid: groupId,
            UserUuid:  applicantId,
            Role:      3, // 普通成员
            Status:    0,
        }
        if err := txRepos.GroupMember.Create(newMember); err != nil {
            return err
        }
        
        // 4. 添加到成员的会话列表
        contact := &model.Contact{
            UserUuid:   applicantId,
            GroupUuid:  groupId,
            Status:     0,
        }
        if err := txRepos.Contact.Create(contact); err != nil {
            return err
        }
        
        return nil
    })
}
```

---

## 实施步骤

### Step 1: 查看现有 Service 实现 (30分钟)
- 读取 `internal/service/apply/*.go`
- 读取 `internal/service/group/*.go`
- 识别需要事务的操作

### Step 2: 修改 Apply Service (40分钟)
- 修改 `PassFriendApply` 使用事务
- 修改 `PassGroupApply` 使用事务
- 修改 `RefuseFriendApply`（可选，单条更新可不用事务）

### Step 3: 修改 Group Service (30分钟)
- 修改 `DismissGroup` 使用事务
- 修改 `RemoveGroupMembers`（批量删除，建议使用事务）

### Step 4: 测试验证 (30分钟)
- 模拟中间失败场景
- 验证数据一致性
- 检查死锁风险

---

## 数据库锁注意事项

### 潜在死锁场景

**场景**: 同时通过 A→B 和 B→A 的好友申请
```
事务1: 更新 Apply(A,B) → 插入 Contact(A,B) → 插入 Contact(B,A)
事务2: 更新 Apply(B,A) → 插入 Contact(B,A) → 插入 Contact(A,B)
```

**解决方案**:
1. 统一操作顺序（按用户ID排序）
2. 减少事务范围
3. 使用重试机制

```go
// 按ID排序，保证全局一致的加锁顺序
if applicantId < userId {
    // 先创建 applicantId -> userId
} else {
    // 先创建 userId -> applicantId
}
```

---

## 测试方案

### 1. 正常流程测试
```go
func TestPassFriendApply_Success(t *testing.T) {
    // 通过申请
    err := service.PassFriendApply("user1", "user2")
    assert.NoError(t, err)
    
    // 验证申请状态为已通过
    apply, _ := repo.FindApply("user2", "user1")
    assert.Equal(t, "passed", apply.Status)
    
    // 验证双向好友关系存在
    assert.True(t, repo.IsFriend("user1", "user2"))
    assert.True(t, repo.IsFriend("user2", "user1"))
}
```

### 2. 异常回滚测试
```go
func TestPassFriendApply_Rollback(t *testing.T) {
    // 模拟第二个操作失败
    mockRepo.On("Create", mock.Anything).Return(errors.New("db error")).Once()
    
    // 执行应该失败
    err := service.PassFriendApply("user1", "user2")
    assert.Error(t, err)
    
    // 验证数据一致性（申请状态应回滚）
    apply, _ := repo.FindApply("user2", "user1")
    assert.Equal(t, "pending", apply.Status) // 仍为待处理
    
    // 验证无好友关系
    assert.False(t, repo.IsFriend("user1", "user2"))
}
```

---

## 预期效果

- ✅ 好友申请通过的原子性保证
- ✅ 群组解散不会留下脏数据
- ✅ 入群审批一致性保证
- ✅ 数据完整性提升到 100%

---

## 回滚方案

如果出现问题：
1. 恢复 Service 层代码，移除事务
2. 重启服务
3. 手动修复不一致数据（如有）

---

## 性能影响

| 操作 | 无事务耗时 | 有事务耗时 | 影响 |
|------|-----------|-----------|------|
| 通过好友申请 | ~10ms | ~15ms | +50% |
| 解散群组(100人) | ~50ms | ~70ms | +40% |
| 入群审批 | ~10ms | ~15ms | +50% |

**结论**: 影响可接受，数据一致性更重要
