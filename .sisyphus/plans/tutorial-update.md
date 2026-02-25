# KamaChat Tutorial 更新计划

## TL;DR

> **快速Summary**: 全面更新 tutorial 目录，使其与实际代码结构完全一致，并补充新增功能文档
> 
> **主要工作**:
> - 更新 README.md 目录 (补充缺失章节)
> - 重写 01_项目初始化与目录结构.md (对齐实际结构)
> - 更新所有过时的章节内容
> - 补充新增功能文档
> 
> **预计工作量**: 大
> **并行执行**: 是

---

## Context

### 原始需求
用户表示 tutorial 目录内容过时，需要全面更新。

### 访谈确认
- **更新范围**: 全面重写
- **更新重点**: 全部文件
- **新增内容**: 需要补充

### 研究发现

**实际项目结构** (internal/ 目录):
| 目录 | 用途 |
|------|------|
| config/ | 配置加载 |
| dao/ | 数据访问层 |
| dto/ | 数据传输对象 |
| handler/ | HTTP 处理器 |
| https_server/ | HTTPS 服务器 |
| infrastructure/ | 基础设施 (日志/中间件/MQ/SMS) |
| model/ | 数据模型 |
| router/ | 路由定义 |
| service/ | 业务逻辑层 |

**DAO 子模块** (internal/dao/mysql/):
- user, friendship, group, member, message, session, apply

**Service 子模块** (internal/service/):
- user, friendship, group, message, session, chat, apply, admin, auth, push

**路由模块** (internal/router/):
- auth_routes.go, user_routes.go, friend_routes.go, group_routes.go
- session_routes.go, message_routes.go, admin_routes.go, ws_routes.go

**新增功能需要补充**:
- 消息撤回 (Message Recall)
- 会话置顶 (Session Pin)
- 群成员禁言 (Group Mute)
- 好友备注 (Friend Remark)
- 管理员功能 (Admin Features)

---

## Work Objectives

### 核心目标
更新 tutorial 目录，使其与实际代码结构完全一致，并补充新增功能文档。

### 具体 deliverables
- [x] 更新后的 README.md (包含所有 27 章)
- [x] 更新的 01_项目初始化与目录结构.md
- [x] 更新的各 API 章节 (11-13, 对应实际路由)
- [x] 新增管理员功能章节
- [x] 新增消息撤回/会话置顶等进阶功能章节

### 定义 of Done
- [ ] README.md 包含章节 01-27
- [ ] 01 章目录结构与实际代码一致
- [ ] 所有过时的模块名称已更新 (contact → friendship)
- [ ] 新增功能有对应教程章节

---

## Execution Strategy

### 任务分解

**Wave 1: 基础更新 (立即开始)**
- Task 1: 更新 README.md 目录结构 (补充 23-27 章)
- Task 2: 重写 01_项目初始化与目录结构.md
- Task 3: 更新 11_用户模块API.md (对齐 user_routes.go)
- Task 4: 重命名 contact → friendship 相关内容

**Wave 2: API 章节更新**
- Task 5: 更新 12_联系人模块API.md → 12_好友模块API.md
- Task 6: 更新 13_群组模块API.md (对齐 group_routes.go)
- Task 7: 更新会话/消息相关章节

**Wave 3: 新增功能文档**
- Task 8: 创建管理员功能教程 (admin_routes.go)
- Task 9: 创建消息撤回功能教程
- Task 10: 创建会话置顶/群成员禁言教程

**Wave 4: 收尾与验证**
- Task 11: 验证所有文件链接正确
- Task 12: 更新目录索引

---

## TODOs

### Wave 1: 基础更新

- [ ] 1. **更新 README.md 目录结构**

  **What to do**:
  - 在 README.md 中添加缺失的章节 23-27 到目录表格
  - 确保章节编号连续

  **References**:
  - tutorial/README.md (当前文件)
  - tutorial/23_JWT认证与单点登录.md (已存在)
  - tutorial/24_雪花算法与分布式ID.md (已存在)
  - tutorial/25_后端服务优化指南.md (已存在)
  - tutorial/26_消息全链路生命周期.md (已存在)
  - tutorial/27_依赖倒置与接口编程实践.md (已存在)

  **Acceptance Criteria**:
  - [ ] README.md 包含 01-27 所有章节

- [ ] 2. **重写 01_项目初始化与目录结构.md**

  **What to do**:
  - 更新目录结构描述以匹配实际代码
  - 将 internal/dao/mysql/repository 改为具体的子模块
  - 将 internal/service/{chat,contact,group...} 展开为实际模块列表
  - 添加 friendship, apply, admin 等新模块说明
  - 更新路由模块说明 (auth_routes, friend_routes 等)

  **References**:
  - internal/ (实际目录结构)
  - internal/service/ (实际 service 模块)
  - internal/router/*.go (实际路由文件)

  **Acceptance Criteria**:
  - [ ] 目录结构与实际完全一致
  - [ ] 包含所有现有模块说明

- [ ] 3. **更新 11_用户模块API.md**

  **What to do**:
  - 检查并更新用户 API 描述
  - 对齐实际的 user_routes.go 路由

  **References**:
  - internal/router/user_routes.go
  - internal/handler/user_handler.go

  **Acceptance Criteria**:
  - [ ] 路由与代码一致

- [ ] 4. **更新 contact → friendship 命名**

  **What to do**:
  - 将 12_联系人模块API.md 重命名为 12_好友模块API.md
  - 更新内部对 "contact" 的引用为 "friendship"

  **References**:
  - internal/service/friendship/
  - internal/handler/friendship_handler.go
  - internal/router/friend_routes.go

  **Acceptance Criteria**:
  - [ ] 文件已重命名
  - [ ] 内容更新为 friendship 术语

### Wave 2: API 章节更新

- [ ] 5. **更新好友模块 API 章节**

  **What to do**:
  - 更新 12_好友模块API.md 内容
  - 对齐 friend_routes.go 中的实际路由

  **References**:
  - internal/router/friend_routes.go
  - internal/handler/friendship_handler.go

  **Acceptance Criteria**:
  - [ ] 路由与代码一致

- [ ] 6. **更新群组模块 API 章节**

  **What to do**:
  - 更新 13_群组模块API.md 内容
  - 补充群成员禁言功能说明

  **References**:
  - internal/router/group_routes.go
  - internal/handler/group_handler.go

  **Acceptance Criteria**:
  - [ ] 包含禁言功能

- [ ] 7. **更新会话/消息章节**

  **What to do**:
  - 检查并更新会话和消息相关章节
  - 补充消息撤回、会话置顶功能

  **References**:
  - internal/router/session_routes.go
  - internal/router/message_routes.go

  **Acceptance Criteria**:
  - [ ] 路由与代码一致

### Wave 3: 新增功能文档

- [ ] 8. **创建管理员功能教程**

  **What to do**:
  - 新建 28_管理员功能.md
  - 包含用户管理、群组管理等管理员操作

  **References**:
  - internal/router/admin_routes.go
  - internal/handler/admin_handler.go
  - internal/service/admin/

  **Acceptance Criteria**:
  - [ ] 文件已创建
  - [ ] 包含管理员路由说明

- [ ] 9. **创建消息撤回功能教程**

  **What to do**:
  - 在消息章节中补充或新建消息撤回教程

  **References**:
  - internal/handler/message_handler.go (RecallMessage 方法)
  - internal/service/message/service.go

  **Acceptance Criteria**:
  - [ ] 包含撤回 API 说明

- [ ] 10. **创建会话置顶/群禁言教程**

  **What to do**:
  - 补充会话置顶 (PinSession) 和群成员禁言 (MuteMember) 功能

  **References**:
  - internal/dto/request/session/pin_session_request.go
  - internal/dto/request/group/mute_member_request.go

  **Acceptance Criteria**:
  - [ ] 包含功能说明

### Wave 4: 收尾与验证

- [ ] 11. **验证文件链接**

  **What to do**:
  - 检查所有教程中的文件路径引用是否正确
  - 确保 "下一步" 链接有效

  **Acceptance Criteria**:
  - [ ] 所有链接有效

- [ ] 12. **更新目录索引**

  **What to do**:
  - 最终确认 README.md 完整性
  - 确保编号连续、内容完整

  **Acceptance Criteria**:
  - [ ] README 完整

---

## Commit Strategy

| After Task | Message | Files |
|------------|---------|-------|
| 1 | docs: 更新 tutorial README 目录 | tutorial/README.md |
| 2 | docs: 重写项目初始化教程 | tutorial/01_*.md |
| 3-4 | docs: 更新用户和好友模块教程 | tutorial/11_*.md, 12_*.md |
| 5-7 | docs: 更新 API 章节 | tutorial/13_*.md 等 |
| 8-10 | docs: 新增功能教程 | tutorial/28_*.md 等 |
| 11-12 | docs: 验证和收尾 | tutorial/*.md |

---

## Success Criteria

- [ ] README.md 包含 01-28 所有章节
- [ ] 目录结构与实际代码一致
- [ ] 模块名称使用正确的术语 (friendship, admin, apply)
- [ ] 新增功能有对应文档
- [ ] 所有内部链接有效
