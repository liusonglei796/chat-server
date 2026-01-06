# 09. Gin 框架搭建与路由

> 本教程将使用 Gin 框架搭建 HTTP 服务器，并设计 RESTful API 路由。

---

## 📌 学习目标

- 理解 Gin 框架核心概念
- 实现 HTTP/HTTPS 服务器
- 掌握模块化路由设计
- 理解 JWT 认证中间件

---

## 1. Gin 框架简介

**Gin** 是 Go 语言最流行的 Web 框架：

| 特性 | 说明 |
|-----|------|
| 高性能 | 基于 httprouter，速度极快 |
| 中间件 | 灵活的中间件机制 |
| 路由分组 | 支持路由分组和版本控制 |
| 参数绑定 | 自动绑定 JSON/Form/Query 参数 |
| 验证器 | 内置参数验证 |

---

## 2. 创建 HTTP 服务器

### 2.1 internal/https_server/https_server.go

> 职责：初始化 Gin 引擎、配置中间件、注册路由。

```go
package https_server

import (
	"kama_chat_server/internal/config"
	"kama_chat_server/internal/infrastructure/logger"
	"kama_chat_server/internal/router"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

var GE *gin.Engine

// Init 初始化 HTTPS 服务器
func Init() {
	GE = gin.New()
	// 使用自定义的 zap logger 和 recovery 中间件
	GE.Use(logger.GinLogger())
	GE.Use(logger.GinRecovery(true))

	// CORS 配置
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"*"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	GE.Use(cors.New(corsConfig))

	// 静态资源
	GE.Static("/static/avatars", config.GetConfig().StaticAvatarPath)
	GE.Static("/static/files", config.GetConfig().StaticFilePath)

	// 注册所有路由
	router.RegisterRoutes(GE)
}
```

**关键点**：
- 使用 `gin.New()` 替代 `gin.Default()`，搭配自定义日志中间件
- 使用 `logger.GinLogger()` 和 `logger.GinRecovery(true)` 中间件
- CORS 配置支持 `Authorization` 头（用于 JWT）
- 静态资源服务用于头像和文件访问

---

## 3. 模块化路由设计

我们使用 `internal/router` 包来统一管理路由，使 `https_server.go` 更加简洁。

### 3.1 internal/router/router.go

```go
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有路由
func RegisterRoutes(r *gin.Engine) {
	RegisterAuthRoutes(r)      // 认证路由（Token 刷新）
	RegisterUserRoutes(r)
	RegisterGroupRoutes(r)
	RegisterContactRoutes(r)
	RegisterSessionRoutes(r)
	RegisterMessageRoutes(r)
	RegisterWebSocketRoutes(r)
	RegisterChatRoomRoutes(r)
}
```

### 3.2 路由模块示例 (internal/router/user_routes.go)

> **重要**：使用 JWT 中间件保护需要认证的接口

```go
package router

import (
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/infrastructure/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes 注册用户相关路由
func RegisterUserRoutes(r *gin.Engine) {
	// 公开接口 (无需认证)
	r.POST("/login", handler.LoginHandler)
	r.POST("/register", handler.RegisterHandler)
	r.POST("/user/smsLogin", handler.SmsLoginHandler)
	r.POST("/user/sendSmsCode", handler.SendSmsCodeHandler)

	// 需要认证的接口
	userGroup := r.Group("/user")
	userGroup.Use(middleware.JWTAuth())
	{
		userGroup.POST("/wsLogout", handler.WsLogoutHandler)
		userGroup.POST("/updateUserInfo", handler.UpdateUserInfoHandler)
		userGroup.GET("/getUserInfoList", handler.GetUserInfoListHandler)
		userGroup.GET("/getUserInfo", handler.GetUserInfoHandler)
		userGroup.POST("/ableUsers", handler.AbleUsersHandler)
		userGroup.POST("/disableUsers", handler.DisableUsersHandler)
		userGroup.POST("/deleteUsers", handler.DeleteUsersHandler)
		userGroup.POST("/setAdmin", handler.SetAdminHandler)
	}
}
```

**设计要点**：
- 公开接口（登录、注册、短信登录）不需要 JWT
- 其他接口使用 `middleware.JWTAuth()` 保护
- 使用路由分组 `r.Group("/user")` 统一添加中间件

### 3.3 认证路由 (internal/router/auth_routes.go)

```go
package router

import (
	"kama_chat_server/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterAuthRoutes 注册认证相关路由
func RegisterAuthRoutes(r *gin.Engine) {
	r.POST("/auth/refreshToken", handler.RefreshTokenHandler)
}
```

---

## 4. Handler (处理器) 实现

所有 Handler 位于 `internal/handler/` 目录下。

> **架构变更说明**：从 `api/v1/` 移至 `internal/handler/`，与路由层解耦。

### 4.1 internal/handler/user_handler.go

```go
package handler

import (
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterHandler 用户注册
func RegisterHandler(c *gin.Context) {
	var req request.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.User.Register(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetUserInfoHandler 获取用户信息
func GetUserInfoHandler(c *gin.Context) {
	var req request.GetUserInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.User.GetUserInfo(req.Uuid)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

**注意**：使用 `service.Svc.User` 调用服务，而不是直接导入 `service/user` 包。

---

## 5. 更新 main.go

更新 `cmd/kama_chat_server/main.go` 以启动 HTTP 服务：

```go
package main

import (
	"fmt"
	"log"

	"go.uber.org/zap"
	"kama_chat_server/internal/config"
	dao "kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/https_server"
	"kama_chat_server/internal/infrastructure/logger"
	"kama_chat_server/internal/service"
)

func main() {
	fmt.Println("KamaChat Server Starting...")

	// 1. 加载配置
	cfg := config.GetConfig()

	// 2. 初始化日志
	if err := logger.Init(&cfg.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}
	defer logger.Sync()

	// 3. 初始化数据库
	dao.Init()
	zap.L().Info("数据库初始化成功")

	// 4. 初始化 Redis
	myredis.Init()
	zap.L().Info("Redis 初始化成功")

	// 5. 初始化 Service 层 (依赖注入)
	service.InitServices(dao.Repos)
	zap.L().Info("Service 层初始化成功")

	// 6. 初始化翻译器
	if err := handler.InitTrans("zh"); err != nil {
		zap.L().Fatal("init translator failed", zap.Error(err))
	}

	// 7. 初始化 HTTPS 服务路由
	https_server.Init()

	// 8. 启动服务
	addr := fmt.Sprintf("%s:%d", cfg.MainConfig.Host, cfg.MainConfig.Port)
	zap.L().Info("HTTP Server starting", zap.String("addr", addr))

	if err := https_server.GE.Run(addr); err != nil {
		zap.L().Fatal("Failed to start HTTP server", zap.Error(err))
	}
}
```

**关键初始化顺序**：
1. 配置 → 2. 日志 → 3. 数据库 → 4. Redis → 5. **Service 层** → 6. 翻译器 → 7. 路由 → 8. 启动

---

## 6. 运行测试

### 6.1 启动服务器

```bash
cd cmd/kama_chat_server
go run main.go
```

### 6.2 测试 API

```bash
# 测试用户注册接口
curl -X POST http://localhost:8000/register \
  -H "Content-Type: application/json" \
  -d '{"telephone":"13800138000","password":"123456","nickname":"测试用户","sms_code":"123456"}'

# 测试登录接口
curl -X POST http://localhost:8000/login \
  -H "Content-Type: application/json" \
  -d '{"telephone":"13800138000","password":"123456"}'

# 使用 Token 获取用户信息
curl http://localhost:8000/user/getUserInfo?uuid=U123456789 \
  -H "Authorization: Bearer <your_access_token>"
```

---

## ✅ 本节完成

你已经完成了：
- [x] Gin HTTP Server 初始化（使用自定义日志中间件）
- [x] 模块化路由设计 (`internal/router`)
- [x] JWT 认证中间件集成
- [x] Handler 层实现 (`internal/handler`)
- [x] 静态资源服务配置
- [x] Service 层依赖注入

---

## 📚 下一步

继续学习 [10_统一响应与错误处理.md](10_统一响应与错误处理.md)，实现统一的响应格式和错误处理。
