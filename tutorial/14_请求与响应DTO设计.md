# 14. 请求与响应 DTO 设计

> 本教程将总结和规范整个项目的 DTO（Data Transfer Object）层设计，确保 API 接口的一致性和安全性。

---

## 📌 学习目标

- 理解 DTO 分层的意义
- 掌握 Gin 的 tag 验证规则
- 设计规范的 Request 和 Response 结构

---

## 1. 为什么需要 DTO

| 层次 | 对象 | 职责 | 示例 |
|-----|------|------|------|
| API 层 | **DTO** | 数据传输、参数验证、字段过滤 | `RegisterRequest`, `LoginRespond` |
| 业务层 | **BO** | 业务逻辑对象（可选） | - |
| 数据层 | **Model** | 数据库映射、数据持久化 | `UserInfo`, `Message` |

**DTO 的作用**：
1. **解耦**：避免数据库模型直接暴露给前端（如密码字段）。
2. **验证**：在入口处统一验证参数合法性。
3. **聚合**：一个接口可能需要返回多个表的数据。
4. **兼容**：后端数据库结构变更时，不影响前端接口格式。

---

## 2. Request DTO 规范

Request DTO 位于 `internal/dto/request` 包下。

### 2.1 命名规范

- 结构体名：`{Action}{Resource}Request`
- 文件名：`{action}_{resource}_request.go`（snake_case）
- 示例：`LoginRequest`、`CreateGroupRequest`

### 2.2 JSON 字段命名

项目使用 **snake_case** 命名风格：

```go
type RegisterRequest struct {
    Telephone string `json:"telephone" binding:"required"`
    Password  string `json:"password" binding:"required,min=6"`
    Nickname  string `json:"nickname" binding:"required"`
    SmsCode   string `json:"sms_code" binding:"required,len=6"`  // snake_case
}
```

### 2.3 常用验证 Tag（Binding）

Gin 使用 `go-playground/validator` 进行验证：

| Tag | 说明 | 示例 |
|-----|------|------|
| `required` | 必填 | `binding:"required"` |
| `omitempty` | 非必填，若有值则验证 | `binding:"omitempty,min=6"` |
| `len` | 固定长度 | `binding:"len=11"` |
| `min/max` | 字符串长度/数字大小范围 | `binding:"min=6,max=20"` |
| `eq` | 等于指定值 | `binding:"eq=1"` |
| `oneof` | 枚举值校验 | `binding:"oneof=0 1 2"` |
| `email` | 邮箱格式 | `binding:"email"` |
| `url` | URL 格式 | `binding:"url"` |

### 2.4 GET 请求的 form 标签

> **重要**：对于 GET 请求，参数通过 URL 查询字符串传递，需要添加 `form` 标签。

```go
// POST 请求 - 只需要 json 标签
type LoginRequest struct {
    Telephone string `json:"telephone" binding:"required"`
}

// GET 请求 - 需要同时添加 json 和 form 标签
type GetUserInfoRequest struct {
    Uuid string `json:"uuid" form:"uuid" binding:"required"`
}
```

在 Handler 中，GET 请求使用 `ShouldBindQuery` 代替 `ShouldBindJSON`：

```go
// GET 请求
func GetUserInfoHandler(c *gin.Context) {
    var req request.GetUserInfoRequest
    if err := c.ShouldBindQuery(&req); err != nil {  // 使用 ShouldBindQuery
        HandleParamError(c, err)
        return
    }
    // ...
}
```

### 2.5 项目实际 Request 示例

```go
// 用户模块
type LoginRequest struct {
    Telephone string `json:"telephone" binding:"required"`
    Password  string `json:"password" binding:"required,min=6"`
}

type SmsLoginRequest struct {
    Telephone string `json:"telephone" binding:"required"`
    SmsCode   string `json:"sms_code" binding:"required,len=6"`
}

// 联系人模块
type ApplyContactRequest struct {
    UserId    string `json:"user_id" binding:"required"`
    ContactId string `json:"contact_id" binding:"required"`
    Message   string `json:"message"`
}

// 群组模块
type CreateGroupRequest struct {
    OwnerId string `json:"owner_id" binding:"required"`
    Name    string `json:"name" binding:"required"`
    Notice  string `json:"notice"`
    AddMode int8   `json:"add_mode"`
    Avatar  string `json:"avatar"`
}

type RemoveGroupMembersRequest struct {
    GroupId  string   `json:"group_id" binding:"required"`
    OwnerId  string   `json:"owner_id" binding:"required"`
    UuidList []string `json:"uuid_list" binding:"required,min=1"`
}

// 消息翻译模块（支持开关 + 目标语言）
type TranslateRequest struct {
    MessageID       string `json:"messageId" binding:"required"`
    EnableTranslate *bool  `json:"enableTranslate"` // nil 表示默认开启（兼容旧客户端）
    TargetLang      string `json:"targetLang"`      // 为空时后端默认“英语”
}
```

---

## 3. Response DTO 规范

Response DTO 位于 `internal/dto/respond` 包下。

### 3.1 命名规范

- 结构体名：`{Resource}Respond` 或 `{Action}Respond`
- 文件名：`{resource}_respond.go`（snake_case）
- 示例：`LoginRespond`、`GetGroupInfoRespond`

### 3.2 字段控制

- 使用 `json` tag 控制输出字段名（snake_case）
- 使用 `omitempty` 隐藏空值字段
- 绝不包含敏感信息（密码、盐值等）
- 时间格式化为字符串（前端友好）

### 3.3 项目实际 Respond 示例

```go
// 用户模块
type LoginRespond struct {
    Uuid         string `json:"uuid"`
    Nickname     string `json:"nickname"`
    Telephone    string `json:"telephone"`
    Avatar       string `json:"avatar"`
    Email        string `json:"email"`
    Gender       int8   `json:"gender"`
    Birthday     string `json:"birthday"`
    Signature    string `json:"signature"`
    CreatedAt    string `json:"created_at"`   // snake_case
    IsAdmin      int8   `json:"is_admin"`     // snake_case
    Status       int8   `json:"status"`
    AccessToken  string `json:"access_token"`  // JWT Access Token
    RefreshToken string `json:"refresh_token"` // JWT Refresh Token
}

// 群组模块
type GetGroupInfoRespond struct {
    Uuid      string `json:"uuid"`
    Name      string `json:"name"`
    Notice    string `json:"notice"`
    MemberCnt int    `json:"member_cnt"`   // snake_case
    OwnerId   string `json:"owner_id"`     // snake_case
    AddMode   int8   `json:"add_mode"`     // snake_case
    Status    int8   `json:"status"`
    Avatar    string `json:"avatar"`
    IsDeleted bool   `json:"is_deleted"`   // snake_case
}

// 消息翻译模块响应（camelCase 与现有翻译接口保持一致）
type TranslateRespond struct {
    TranslationEnabled bool   `json:"translationEnabled"`
    TargetLang         string `json:"targetLang"`
    OriginalText       string `json:"originalText"`
    TranslatedText     string `json:"translatedText"`
}
```

---

## 4. DTO 转换方式

### 4.1 手动转换（推荐）

Go 语言推荐显式转换，清晰且性能好：

```go
// Model -> Respond
func ToLoginRespond(user model.UserInfo) respond.LoginRespond {
    rsp := respond.LoginRespond{
        Uuid:      user.Uuid,
        Nickname:  user.Nickname,
        Telephone: user.Telephone,
        // ...
    }
    return rsp
}
```

---

## 5. 项目 DTO 索引

### 5.1 用户模块 (`internal/dto/request/`)

| 文件 | 结构体 | 说明 |
|------|--------|------|
| `register_request.go` | `RegisterRequest` | 用户注册 |
| `login_request.go` | `LoginRequest` | 密码登录 |
| `sms_login_request.go` | `SmsLoginRequest` | 验证码登录 |
| `update_userinfo_request.go` | `UpdateUserInfoRequest` | 更新用户信息 |
| `get_userinfo_request.go` | `GetUserInfoRequest` | 获取用户信息 |
| `able_users_request.go` | `AbleUsersRequest` | 启用/禁用用户 |

### 5.2 联系人模块

| 文件 | 结构体 | 说明 |
|------|--------|------|
| `ownlist_request.go` | `OwnlistRequest` | 获取列表通用请求 |
| `apply_contact_request.go` | `ApplyContactRequest` | 申请添加联系人 |
| `pass_contact_apply_request.go` | `PassContactApplyRequest` | 通过/拒绝申请 |
| `delete_contact_request.go` | `DeleteContactRequest` | 删除联系人 |
| `black_contact_request.go` | `BlackContactRequest` | 拉黑联系人 |

### 5.3 群组模块

| 文件 | 结构体 | 说明 |
|------|--------|------|
| `create_group_request.go` | `CreateGroupRequest` | 创建群组 |
| `update_groupinfo_request.go` | `UpdateGroupInfoRequest` | 更新群信息 |
| `leave_group_request.go` | `LeaveGroupRequest` | 退出群组 |
| `dismiss_group_request.go` | `DismissGroupRequest` | 解散群组 |
| `remove_groupmembers_request.go` | `RemoveGroupMembersRequest` | 移除群成员 |

---

## ✅ 本节完成

你已经完成了：
- [x] Request DTO 验证规则
- [x] Response DTO 字段规范
- [x] snake_case JSON 字段命名规范
- [x] DTO 设计模式总结

---

## 📚 阶段三完成！

恭喜！你已经完成了 **阶段三：HTTP API 服务**。

继续学习 [15_WebSocket基础与连接管理.md](15_WebSocket基础与连接管理.md)，开启核心的 **阶段五：WebSocket 实时通讯**。
