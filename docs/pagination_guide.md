# 分页最佳实践指南

本文档介绍 KamaChat 项目中分页的标准实现模式，帮助开发者理解并正确实现分页功能。

## 内存分页 vs 数据库分页

### ❌ 内存分页（不推荐）

```go
// 错误示范：先查全部，再在内存中切片
func GetList(page, pageSize int) ([]Item, int64, error) {
    // 1. 查询所有数据 - 问题根源！
    allItems, err := repo.FindAll()
    total := int64(len(allItems))
    
    // 2. 内存分页
    start := (page - 1) * pageSize
    end := start + pageSize
    if end > len(allItems) {
        end = len(allItems)
    }
    
    return allItems[start:end], total, nil
}
```

**问题**：
- 数据量大时内存压力大
- 数据库到应用的传输开销大
- 无法利用数据库索引优化

### ✅ 数据库分页（推荐）

```go
// 正确示范：在数据库层面分页
func GetList(page, pageSize int) ([]Item, int64, error) {
    // 直接从数据库获取当前页数据
    items, total, err := repo.FindAllPaged(page, pageSize)
    return items, total, err
}
```

---

## 标准实现模式

### DAO 层

```go
// 分页方法签名标准：返回 (数据列表, 总数, 错误)
func (r *repository) FindAllPaged(page, pageSize int) ([]Model, int64, error) {
    var items []Model
    var total int64
    
    // 1. 统计总数
    if err := r.db.Model(&Model{}).Count(&total).Error; err != nil {
        return nil, 0, err
    }
    
    // 2. 分页查询
    offset := (page - 1) * pageSize
    if err := r.db.Order("created_at DESC").
        Offset(offset).
        Limit(pageSize).
        Find(&items).Error; err != nil {
        return nil, 0, err
    }
    
    return items, total, nil
}
```

### Service 层

```go
func (s *service) GetList(page, pageSize int) ([]Response, int64, error) {
    // 1. 参数校验
    if page < 1 {
        page = 1
    }
    if pageSize < 1 || pageSize > 100 {
        pageSize = 20
    }
    
    // 2. 调用 DAO 分页方法
    items, total, err := s.repos.Item.FindAllPaged(page, pageSize)
    if err != nil {
        return nil, 0, err
    }
    
    // 3. 转换为响应对象
    result := make([]Response, 0, len(items))
    for _, item := range items {
        result = append(result, toResponse(item))
    }
    
    return result, total, nil
}
```

---

## 性能对比

| 数据量 | 内存分页耗时 | 数据库分页耗时 |
|--------|-------------|---------------|
| 100条  | ~5ms        | ~3ms          |
| 1000条 | ~50ms       | ~5ms          |
| 10000条| ~500ms      | ~8ms          |

---

## 项目中的分页方法

### 已实现数据库分页的方法

| 模块 | 方法 | DAO 方法 |
|------|------|----------|
| Session | `GetUserSessionList` | `FindBySendIdPaged` |
| Message | `GetGroupMessageList` | `FindByGroupIdPaged` |
| Contact | `GetUserList` | `FindByUserIdAndType` |
| GroupMember | `GetGroupMemberList` | `FindMembersWithUserInfoPaged` |
| Apply | `GetFriendApplyList` | `FindByTargetIdPendingPaged` |
| Apply | `GetGroupApplyList` | `FindByTargetIdPendingPaged` |
| Group | `LoadMyGroup` | `FindByOwnerIdPaged` |

---

## 命名规范

- 分页方法后缀：`*Paged`（如 `FindByUserIdPaged`）
- 返回值顺序：`(数据列表, 总数, 错误)`
- 参数顺序：业务参数在前，`page, pageSize` 在后
