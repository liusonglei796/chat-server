# KamaChat 教程同步计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 30 篇教程文档与当前代码库同步，确保文档描述与实际实现一致

**Architecture:** 这是一个文档更新任务，需要逐个对比教程内容与代码实现，找出差异并更新文档

**Tech Stack:** Go, Markdown, 文件编辑工具

---

## 当前代码库结构分析

### internal/ 目录
```
internal/
├── config/              # 配置管理
├── dao/
│   ├── mysql/           # MySQL 访问层
│   │   ├── init_mysql.go
│   │   ├── interfaces.go
│   │   ├── provider.go
│   │   ├── user/
│   │   ├── friendship/
│   │   ├── group/
│   │   ├── message/
│   │   ├── session/
│   │   └── apply/
│   └── redis/
│       ├── redis.go
│       └── cache/
│           ├── helper.go
│           ├── ttl.go
│           └── ttl_test.go
├── dto/
│   ├── request/         # 40 个请求 DTO
│   └── respond/         # 响应 DTO
├── handler/             # HTTP 处理器
│   ├── user_handler.go
│   ├── auth_handler.go
│   ├── friendship_handler.go
│   ├── group_handler.go
│   ├── message_handler.go
│   ├── session_handler.go
│   ├── apply_handler.go
│   ├── admin_handler.go
│   ├── ws_handler.go
│   ├── response.go
│   ├── validator.go
│   └── provider.go
├── https_server/        # HTTPS 服务器
├── infrastructure/     # 基础设施
│   ├── logger/
│   ├── middleware/
│   │   ├── jwt_middleware.go
│   │   ├── rate_limit.go
│   │   ├── https_middleware.go
│   │   └── admin_middleware.go
│   ├── sms/
│   └── snowflake/
├── model/              # 数据模型
│   ├── user_info.go
│   ├── friendship.go
│   ├── group_info.go
│   ├── group_member.go
│   ├── message.go
│   ├── session.go
│   └── apply.go
├── router/             # 路由定义
│   ├── router.go
│   ├── user_routes.go
│   ├── auth_routes.go
│   ├── friend_routes.go
│   ├── group_routes.go
│   ├── message_routes.go
│   ├── session_routes.go
│   ├── admin_routes.go
│   └── ws_routes.go
└── service/            # 业务逻辑层
    ├── interfaces.go
    ├── provider.go
    ├── user/
    ├── auth/
    ├── friendship/
    ├── group/
    ├── message/
    ├── session/
    ├── apply/
    ├── admin/user/
    ├── admin/group/
    └── chat/
        ├── server.go
        ├── ws_gateway.go
        ├── kafka_broker.go
        └── kafka_client.go
```

### pkg/ 目录
```
pkg/
├── errorx/errorx.go
├── constants/constants.go
├── jwt/jwt.go
├── random/random_int.go
└── enum/
    ├── user/user_status/
    ├── message/message_type/
    ├── message/message_status/
    ├── group/group_status/
    ├── group/add_mode/
    ├── friendship/friendship_status/
    ├── apply/apply_type/
    └── apply/apply_status/
```

### Service 接口完整列表 (internal/service/interfaces.go)

**UserService:**
- Login, SmsLogin, SendSmsCode, Register
- UpdateUserInfo, GetUserInfo, GetPublicUserInfo

**SessionService:**
- CheckOpenSessionAllowed, OpenSession
- GetUserSessionList, GetGroupSessionList
- DeleteSession, PinSession

**GroupService:**
- CreateGroup, LoadMyGroup, GetGroupListByMember
- GetGroupDetail, CheckGroupAddMode
- LeaveGroup, DismissGroup
- UpdateGroupInfo, GetGroupMemberList
- RemoveGroupMembers, MuteMember

**FriendshipService:**
- GetFriendList, GetFriendInfo
- DeleteFriend
- BlackFriend, UnblackFriend
- UpdateRemark

**ApplyService:**
- ApplyFriend, GetFriendApplyList
- PassFriendApply, RefuseFriendApply, BlackFriendApply
- ApplyGroup, GetGroupApplyList
- PassGroupApply, RefuseGroupApply, BlackGroupApply

**MessageService:**
- GetMessageList, GetGroupMessageList
- UploadAvatar, UploadFile
- RecallMessage

**AuthService:**
- ValidateTokenID, GetUserIsAdmin

**UserAdminService:**
- GetUserListPaged, BatchUpdateUserStatus, SetAdmin

**GroupAdminService:**
- GetGroupInfoList, DeleteGroups, SetGroupsStatus

---

## 同步任务清单

### Task 1: 教程 01 - 项目初始化与目录结构

**Files:**
- Modify: `tutorial/01_项目初始化与目录结构.md`
- Check: `internal/service/interfaces.go` (已读取)
- Check: `internal/dao/mysql/interfaces.go`
- Check: `internal/dao/redis/redis.go`

**Step 1: 分析当前目录结构差异**

教程描述的目录结构与实际代码库的差异：
1. internal/dao/ 目录结构：教程提到 `dao/mysql` 和 `dao/redis`，实际是平级的
2. internal/service/chat/ 目录：教程提到多个 broker 文件，需要确认
3. pkg/enum/ 目录结构：教程提到按功能分目录，实际也是分目录
4. 需要确认是否有 `internal/infrastructure/middleware/rate_limit.go` 和 `admin_middleware.go`

**Step 2: 运行命令确认目录结构**

```bash
# 确认 internal 子目录
ls -la internal/

# 确认 service 子目录
ls -la internal/service/

# 确认 handler 子目录
ls -la internal/handler/
```

**Step 3: 更新教程文档**

根据实际目录结构更新 `01_项目初始化与目录结构.md`:
- 更新 internal/dao/ 目录结构说明
- 更新 internal/service/ 目录结构，添加 admin/user 和 admin/group
- 添加 internal/infrastructure/middleware/ 的新中间件
- 更新 pkg/enum/ 目录结构

**Step 4: Commit**

```bash
git add tutorial/01_项目初始化与目录结构.md
git commit -m "docs: update tutorial 01 directory structure"
```

---

### Task 2: 教程 02-05 - 基础设施教程

**Files:**
- Modify: `tutorial/02_配置管理系统.md`
- Modify: `tutorial/03_日志系统搭建.md`
- Modify: `tutorial/04_数据库连接.md`
- Modify: `tutorial/05_Redis缓存集成.md`

**Step 1: 读取现有教程内容**

```bash
# 读取各教程文件
cat tutorial/02_配置管理系统.md
cat tutorial/03_日志系统搭建.md
cat tutorial/04_数据库连接.md
cat tutorial/05_Redis缓存集成.md
```

**Step 2: 检查对应代码实现**

```bash
# 检查配置实现
cat internal/config/config.go

# 检查日志实现
ls -la internal/infrastructure/logger/

# 检查数据库实现
cat internal/dao/mysql/init_mysql.go

# 检查 Redis 实现
cat internal/dao/redis/redis.go
```

**Step 3: 更新文档**

检查以下内容是否需要更新：
- 配置字段是否与代码一致
- 日志组件是否有新增功能
- 数据库连接配置是否完整
- Redis 缓存实现是否有变化

**Step 4: Commit**

```bash
git add tutorial/02_配置管理系统.md tutorial/03_日志系统搭建.md tutorial/04_数据库连接.md tutorial/05_Redis缓存集成.md
git commit -m "docs: update infrastructure tutorials (02-05)"
```

---

### Task 3: 教程 06-08 - 数据模型教程

**Files:**
- Modify: `tutorial/06_用户模型设计.md`
- Modify: `tutorial/07_联系人与群组模型.md`
- Modify: `tutorial/08_会话与消息模型.md`

**Step 1: 检查 Model 定义**

```bash
# 读取所有模型文件
cat internal/model/user_info.go
cat internal/model/friendship.go
cat internal/model/group_info.go
cat internal/model/group_member.go
cat internal/model/message.go
cat internal/model/session.go
cat internal/model/apply.go
```

**Step 2: 对比教程内容**

检查模型字段、注释、GORM 标签是否与教程描述一致

**Step 3: 更新文档**

确保教程中的模型定义与实际代码一致

**Step 4: Commit**

```bash
git add tutorial/06_用户模型设计.md tutorial/07_联系人与群组模型.md tutorial/08_会话与消息模型.md
git commit -m "docs: update model tutorials (06-08)"
```

---

### Task 4: 教程 09-13 - API 教程

**Files:**
- Modify: `tutorial/09_Gin框架搭建与路由.md`
- Modify: `tutorial/10_统一响应与错误处理.md`
- Modify: `tutorial/11_用户模块API.md`
- Modify: `tutorial_12_好友模块API.md`
- Modify: `tutorial_13_群组模块API.md`

**Step 1: 检查 Handler 和 Router 实现**

```bash
# 检查路由
cat internal/router/router.go
ls internal/router/

# 检查 Handler
ls internal/handler/
cat internal/handler/user_handler.go
cat internal/handler/friendship_handler.go
cat internal/handler/group_handler.go
```

**Step 2: 检查 DTO 定义**

```bash
# 列出所有请求 DTO
ls -la internal/dto/request/auth/
ls -la internal/dto/request/friendship/
ls -la internal/dto/request/group/
```

**Step 3: 对比 API 接口**

根据 `internal/service/interfaces.go` 中的接口定义，检查教程中描述的 API 是否完整准确

**Step 4: 更新文档**

更新 API 路由、请求参数、响应格式

**Step 5: Commit**

```bash
git add tutorial/09_Gin框架搭建与路由.md tutorial/10_统一响应与错误处理.md tutorial/11_用户模块API.md tutorial/12_好友模块API.md tutorial/13_群组模块API.md
git commit -m "docs: update API tutorials (09-13)"
```

---

### Task 5: 教程 14 - DTO 教程

**Files:**
- Modify: `tutorial/14_请求与响应DTO设计.md`

**Step 1: 统计实际 DTO 数量**

```bash
# 统计请求 DTO
find internal/dto/request -name "*.go" | wc -l

# 统计响应 DTO
find internal/dto/respond -name "*.go" | wc -l

# 列出所有 DTO
ls -R internal/dto/request/
ls -R internal/dto/respond/
```

**Step 2: 更新文档**

更新 DTO 目录结构和数量

**Step 3: Commit**

```bash
git add tutorial/14_请求与响应DTO设计.md
git commit -m "docs: update DTO tutorial 14"
```

---

### Task 6: 教程 15-17 - WebSocket 教程

**Files:**
- Modify: `tutorial/15_WebSocket基础与连接管理.md`
- Modify: `tutorial/16_聊天服务器实现.md`
- Modify: `tutorial/17_单聊与群聊消息处理.md`

**Step 1: 检查 WebSocket 实现**

```bash
# 检查 WebSocket Handler
cat internal/handler/ws_handler.go

# 检查 Chat Service
ls internal/service/chat/
cat internal/service/chat/server.go
cat internal/service/chat/ws_gateway.go
```

**Step 2: 对比教程内容**

检查消息处理逻辑是否与教程描述一致

**Step 3: 更新文档**

**Step 4: Commit**

```bash
git add tutorial/15_WebSocket基础与连接管理.md tutorial/16_聊天服务器实现.md tutorial/17_单聊与群聊消息处理.md
git commit -m "docs: update WebSocket tutorials (15-17)"
```

---

### Task 7: 教程 18 - Kafka 教程

**Files:**
- Modify: `tutorial/18_Kafka集成与消息模式.md`

**Step 1: 检查 Kafka 实现**

```bash
# 检查 Kafka Broker
cat internal/service/chat/kafka_broker.go
cat internal/service/chat/kafka_client.go
```

**Step 2: 更新文档**

确保 Kafka 配置和消息模式与代码一致

**Step 3: Commit**

```bash
git add tutorial/18_Kafka集成与消息模式.md
git commit -m "docs: update Kafka tutorial 18"
```

---

### Task 8: 教程 19-22 - 进阶功能

**Files:**
- Modify: `tutorial/19_短信验证码服务.md`
- Modify: `tutorial_20_文件上传与静态资源.md`
- Modify: `tutorial_21_HTTPS与安全配置.md`
- Modify: `tutorial_22_音视频通话信令转发.md`

**Step 1: 检查实现**

```bash
# 检查短信服务
cat internal/infrastructure/sms/auth_code_service.go

# 检查文件上传
grep -r "UploadAvatar\|UploadFile" internal/service/message/

# 检查 HTTPS
cat internal/https_server/https_server.go

# 检查信令转发
grep -r "call\|video\| signaling" internal/service/chat/
```

**Step 2: 更新文档**

**Step 3: Commit**

```bash
git add tutorial/19_短信验证码服务.md tutorial/20_文件上传与静态资源.md tutorial/21_HTTPS与安全配置.md tutorial/22_音视频通话信令转发.md
git commit -m "docs: update advanced tutorials (19-22)"
```

---

### Task 9: 教程 23-24 - 认证与安全

**Files:**
- Modify: `tutorial_23_JWT认证与单点登录.md`
- Modify: `tutorial_24_雪花算法与分布式ID.md`

**Step 1: 检查实现**

```bash
# 检查 JWT 实现
cat pkg/jwt/jwt.go
cat internal/infrastructure/middleware/jwt_middleware.go
cat internal/service/auth/service.go

# 检查雪花算法
cat internal/infrastructure/snowflake/snowflake.go
```

**Step 2: 更新文档**

**Step 3: Commit**

```bash
git add tutorial/23_JWT认证与单点登录.md tutorial/24_雪花算法与分布式ID.md
git commit -m "docs: update auth tutorials (23-24)"
```

---

### Task 10: 教程 25-26 - 性能优化

**Files:**
- Modify: `tutorial_25_后端服务优化指南.md`
- Modify: `tutorial_26_消息全链路生命周期.md`

**Step 1: 检查现有文档内容**

**Step 2: 更新文档**

**Step 3: Commit**

```bash
git add tutorial/25_后端服务优化指南.md tutorial/26_消息全链路生命周期.md
git commit -m "docs: update performance tutorials (25-26)"
```

---

### Task 11: 教程 27 - 架构设计

**Files:**
- Modify: `tutorial_27_依赖倒置与接口编程实践.md`

**Step 1: 检查接口定义**

```bash
# 读取 Service 接口
cat internal/service/interfaces.go

# 读取 DAO 接口
cat internal/dao/mysql/interfaces.go
```

**Step 2: 更新文档**

**Step 3: Commit**

```bash
git add tutorial/27_依赖倒置与接口编程实践.md
git commit -m "docs: update architecture tutorial 27"
```

---

### Task 12: 教程 28-30 - 进阶功能二

**Files:**
- Modify: `tutorial_28_管理员功能.md`
- Modify: `tutorial_29_消息撤回功能.md`
- Modify: `tutorial_30_会话置顶与群成员禁言.md`

**Step 1: 检查实现**

```bash
# 检查管理员功能
cat internal/service/admin/user/service.go
cat internal/service/admin/group/service.go

# 检查消息撤回
grep -r "Recall" internal/service/message/

# 检查会话置顶和禁言
grep -r "Pin\|Mute" internal/service/
```

**Step 2: 更新文档**

**Step 3: Commit**

```bash
git add tutorial/28_管理员功能.md tutorial/29_消息撤回功能.md tutorial/30_会话置顶与群成员禁言.md
git commit -m "docs: update advanced feature tutorials (28-30)"
```

---

### Task 13: 更新教程 README 索引

**Files:**
- Modify: `tutorial/README.md`

**Step 1: 读取当前 README**

```bash
cat tutorial/README.md
```

**Step 2: 更新目录索引**

确保所有教程条目准确

**Step 3: Commit**

```bash
git add tutorial/README.md
git commit -m "docs: update tutorial README index"
```

---

## 执行总结

**总任务数**: 13 个 Task
**预计时间**: 2-3 小时
**提交次数**: 13 次（每个 Task 一次提交）

**关键检查点**:
1. 所有 Service 接口方法都已文档化
2. 所有 Handler 路由与教程一致
3. 所有 Model 字段与数据库表对应
4. DTO 数量和结构准确
5. README 索引完整
