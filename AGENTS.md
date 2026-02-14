# AGENTS.md - KamaChat Server Coding Guidelines

This file provides instructions for AI coding agents working on the KamaChat Server codebase.

## Project Overview

- **Language**: Go 1.20
- **Framework**: Gin v1.10.0
- **Architecture**: Clean Architecture with Handler/Service/DAO layers
- **Database**: MySQL 8.0 (GORM), Redis v8
- **Message Queue**: Kafka (optional) or Go Channel

## Build Commands

```bash
# Download dependencies
go mod download

# Build the application
go build -o kama_chat_server cmd/kama_chat_server/main.go

# Run the application
go run cmd/kama_chat_server/main.go

# Run all tests
go test ./...

# Run a single test
go test -run TestFunctionName ./path/to/package

# Run tests with coverage
go test -cover ./...

# Format code
go fmt ./...

# Vet code (static analysis)
go vet ./...

# Tidy dependencies
go mod tidy

# Verify dependencies
go mod verify
```

## Code Style Guidelines

### Imports Order

Imports must be grouped in this order with blank lines between groups:

1. Standard library imports
2. Third-party imports
3. Internal project imports (prefixed with `kama_chat_server/`)

```go
import (
    "context"
    "fmt"

    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
    "gorm.io/gorm"

    "kama_chat_server/internal/model"
    "kama_chat_server/pkg/errorx"
)
```

### Naming Conventions

- **Packages**: lowercase, single word (e.g., `handler`, `service`, `user`)
- **Exported types/functions**: PascalCase (e.g., `UserHandler`, `NewUserService`)
- **Unexported types/functions**: camelCase (e.g., `checkTelephoneValid`)
- **Interfaces**: Service interfaces end with `Service` (e.g., `UserService`)
- **Request DTOs**: Suffix `Request` (e.g., `LoginRequest`)
- **Response DTOs**: Suffix `Respond` (e.g., `LoginRespond`)
- **File names**: snake_case for multi-word files (e.g., `user_handler.go`)

### Package Comments

Every file must start with a package comment explaining its purpose:

```go
// Package handler 提供 HTTP 请求处理器
// 本文件处理用户相关的 API 请求
package handler
```

### Function Documentation

All exported functions must have comments in Chinese:

```go
// Register 用户注册
// POST /user/register
// 请求体: auth.RegisterRequest
// 响应: respond.RegisterRespond (用户信息)
func (h *UserHandler) Register(c *gin.Context) {
    // implementation
}
```

### Error Handling

Always use the `pkg/errorx` package for errors:

```go
// Create business errors
errorx.New(errorx.CodeNotFound, "用户不存在")
errorx.Newf(errorx.CodeInvalidParam, "参数 %s 无效", paramName)

// Wrap existing errors
errorx.Wrap(err, errorx.CodeDBError, "数据库查询失败")
errorx.Wrapf(err, errorx.CodeDBError, "查询用户 %s 失败", userId)

// Wrap database errors (auto-converts gorm.ErrRecordNotFound)
errorx.WrapDBError(err, "查询用户失败")
```

### Handler Pattern

Handlers follow a 3-step pattern:

```go
func (h *UserHandler) SomeHandler(c *gin.Context) {
    // 1. 绑定并验证请求参数
    var req somepkg.SomeRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        HandleParamError(c, err)
        return
    }

    // 2. 调用 Service 层处理业务逻辑
    data, err := h.userSvc.SomeMethod(req)
    if err != nil {
        HandleError(c, err)
        return
    }

    // 3. 返回成功响应
    HandleSuccess(c, data)
}
```

### Dependency Injection

Dependencies are injected via constructors:

```go
// Handler constructor
func NewUserHandler(userSvc service.UserService) *UserHandler {
    return &UserHandler{userSvc: userSvc}
}

// Service constructor
func NewUserService(repos *mysql.Repositories, cache myredis.CacheService) *userService {
    return &userService{repos: repos, cache: cache}
}
```

### GORM Model Definition

Models use GORM with detailed comments:

```go
// UserInfo 用户信息模型
// 对应数据库 user_info 表
type UserInfo struct {
    gorm.Model
    
    // Uuid 用户唯一标识
    // 格式：U + 13位时间戳随机字符串
    Uuid string `gorm:"column:uuid;uniqueIndex;type:char(20);comment:用户唯一id"`
    
    Nickname string `gorm:"column:nickname;type:varchar(20);not null;comment:昵称"`
}

func (UserInfo) TableName() string {
    return "user_info"
}
```

### Service Return Values

Services return `ret = 0` for success, `ret = -1` for system error, `ret = -2` for business error:

```go
// Return values: (data, error)
// - error == nil: success
// - error is CodeError with Code != CodeServerBusy/CodeDBError: business error
func (s *someService) SomeMethod(req SomeRequest) (*SomeRespond, error) {
    // ...
}
```

### DTO Organization

- Request DTOs: `internal/dto/request/{module}/`
- Response DTOs: `internal/dto/respond/{module}/`
- Use named imports for conflicting package names: `userreq "kama_chat_server/internal/dto/request/user"`

### Logging

Use Zap logger with structured fields:

```go
zap.L().Info("操作成功", zap.String("userId", userId))
zap.L().Error("数据库错误", zap.Error(err), zap.String("sql", sql))
zap.L().Fatal("启动失败", zap.Error(err))
```

### Security Considerations

- Never expose internal error details to clients
- Always validate user ID from JWT context, never trust client-provided IDs
- Use bcrypt for password hashing
- Use UUIDs for user-facing identifiers

### Testing

```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/service/...

# Run a specific test
go test -run TestUserService_Login ./internal/service/user/

# Run with verbose output
go test -v ./...

# Run with race detection
go test -race ./...
```

## Architecture Layers

1. **Handler** (`internal/handler/`): HTTP request handling, parameter validation
2. **Service** (`internal/service/`): Business logic, transaction management
3. **DAO** (`internal/dao/`): Data access, Repository pattern for MySQL + Redis
4. **Model** (`internal/model/`): Database models
5. **DTO** (`internal/dto/`): Request/Response structures

## Common Patterns

- Repository pattern for database access
- Dependency injection via constructors
- Interface-based design for testability
- Custom error type with error codes
- Structured logging with Zap
