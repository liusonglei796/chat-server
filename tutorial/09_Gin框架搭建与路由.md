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
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/infrastructure/logger"
	"kama_chat_server/internal/router"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Init 初始化 HTTP/HTTPS 服务器并返回 Gin 引擎实例
// handlers: 通过依赖注入传入的 handler 聚合对象
func Init(handlers *handler.Handlers) *gin.Engine {
	engine := gin.New()
	// 使用自定义的 zap logger 和 recovery 中间件
	engine.Use(logger.GinLogger())
	engine.Use(logger.GinRecovery(true))

	// CORS 配置
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"*"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	engine.Use(cors.New(corsConfig))

	// 静态资源
	engine.Static("/static/avatars", config.GetConfig().StaticAvatarPath)
	engine.Static("/static/files", config.GetConfig().StaticFilePath)

	// 注册所有路由（通过 Router 对象封装注册逻辑）
	rt := router.NewRouter(handlers)
	rt.RegisterRoutes(engine)

	return engine
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
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/infrastructure/middleware"

	"github.com/gin-gonic/gin"
)

// Router 路由管理器：封装所有路由注册逻辑，通过依赖注入接收 handlers
type Router struct {
	handlers *handler.Handlers
}

func NewRouter(handlers *handler.Handlers) *Router {
	return &Router{handlers: handlers}
}

// RegisterRoutes 注册所有路由
// 路由分为两组：
//   - 公开路由: 无需认证，用于登录、注册、Token 刷新
//   - 私有路由: 需要 JWT 认证
func (rt *Router) RegisterRoutes(r *gin.Engine) {
	public := r.Group("")
	{
		rt.RegisterAuthRoutes(public)
		rt.RegisterPublicUserRoutes(public)
	}

	private := r.Group("")
	private.Use(middleware.JWTAuth())
	{
		rt.RegisterAdminRoutes(private)
		rt.RegisterUserRoutes(private)
		rt.RegisterFriendRoutes(private)
		rt.RegisterGroupRoutes(private)
		rt.RegisterSessionRoutes(private)
		rt.RegisterMessageRoutes(private)
		rt.RegisterWebSocketRoutes(private)
	}
}
```

### 3.2 路由模块示例 (internal/router/user_routes.go)

> **重要**：使用 JWT 中间件保护需要认证的接口

```go
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterPublicUserRoutes 注册用户公开路由（无需认证）
func (rt *Router) RegisterPublicUserRoutes(rg *gin.RouterGroup) {
	rg.POST("/login", rt.handlers.User.Login)
	rg.POST("/register", rt.handlers.User.Register)
	rg.POST("/user/smsLogin", rt.handlers.User.SmsLogin)
	rg.POST("/user/sendSmsCode", rt.handlers.User.SendSmsCode)
}

// RegisterUserRoutes 注册用户相关路由（需要认证）
func (rt *Router) RegisterUserRoutes(rg *gin.RouterGroup) {
	userGroup := rg.Group("/user")
	{
		userGroup.POST("/wsLogout", rt.handlers.Ws.WsLogoutHandler)
		userGroup.POST("/updateUserInfo", rt.handlers.User.UpdateUserInfo)
		userGroup.GET("/getUserInfo", rt.handlers.User.GetUserInfo)
	}
}
```

**设计要点**：
- 公开接口（登录、注册、短信登录）不需要 JWT
- 私有接口统一在 `router.Router.RegisterRoutes()` 中使用 `middleware.JWTAuth()` 保护
- 使用路由分组 `r.Group("/user")` 组织子路由

### 3.3 认证路由 (internal/router/auth_routes.go)

```go
package router

import (
	"github.com/gin-gonic/gin"
)

// RegisterAuthRoutes 注册认证相关路由
func (rt *Router) RegisterAuthRoutes(rg *gin.RouterGroup) {
	authGroup := rg.Group("/auth")
	{
		authGroup.POST("/refresh", rt.handlers.Auth.RefreshToken)
	}
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

type UserHandler struct {
	userSvc service.UserService
}

func NewUserHandler(userSvc service.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

// Register 用户注册
func (h *UserHandler) Register(c *gin.Context) {
	var req request.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.userSvc.Register(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}

// GetUserInfo 获取用户信息
func (h *UserHandler) GetUserInfo(c *gin.Context) {
	var req request.GetUserInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.userSvc.GetUserInfo(req.Uuid)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

**注意**：当前仓库使用构造函数注入 `service.UserService`，通过 `handler.NewHandlers(services, broker)` 聚合后在路由层引用具体方法。

---

## 5. 更新 main.go

更新 `cmd/kama_chat_server/main.go` 以启动 HTTP 服务：

```go
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"kama_chat_server/internal/config"
	dao "kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/https_server"
	"kama_chat_server/internal/infrastructure/logger"
	"kama_chat_server/internal/infrastructure/sms"
	"kama_chat_server/internal/service"
	"kama_chat_server/internal/service/chat"
	"kama_chat_server/pkg/util/jwt"
)

func main() {
	// 1. 加载配置
	conf := config.GetConfig()

	// 2. 初始化日志
	if err := logger.Init(&conf.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}
	zap.L().Info("日志初始化成功")

	// 3. 初始化数据库
	repos := dao.Init()
	zap.L().Info("数据库初始化成功")

	// 4. 初始化 Redis
	cacheService := myredis.Init()
	zap.L().Info("Redis 初始化成功")

	// 5. 初始化 JWT
	jwt.Init(conf.JWTConfig.Secret, conf.JWTConfig.AccessTokenExpiry, conf.JWTConfig.RefreshTokenExpiry)
	zap.L().Info("JWT 初始化成功")

	// 6. 初始化 Service 层 (依赖注入)
	services := service.NewServices(repos, cacheService)
	zap.L().Info("Service 层初始化成功")

	// 7. 初始化 ChatServer（依赖注入）
	chatServer := chat.NewChatServer(chat.ChatServerConfig{
		Mode:            conf.KafkaConfig.MessageMode,
		MessageRepo:     repos.Message,
		GroupMemberRepo: repos.GroupMember,
		CacheService:    cacheService,
	})
	if conf.KafkaConfig.MessageMode == "kafka" {
		chatServer.InitKafka()
	}
	zap.L().Info("ChatServer 初始化成功")

	// 8. 初始化 Handler 层 (依赖注入，包含 ChatServer 的 broker)
	handlers := handler.NewHandlers(services, chatServer.GetBroker())
	zap.L().Info("Handler 层初始化成功")

	// 9. 初始化 SMS Service (依赖注入缓存服务)
	if err := sms.Init(cacheService); err != nil {
		zap.L().Fatal("SMS Service 初始化失败", zap.Error(err))
	}
	zap.L().Info("SMS Service 初始化成功")

	// 10. 初始化 HTTPS 服务器 (传入 handlers 进行依赖注入)
	engine := https_server.Init(handlers)
	zap.L().Info("HTTPS 服务器初始化成功")

	// 11. 启动服务
	host := conf.MainConfig.Host
	port := conf.MainConfig.Port

	// 启动聊天服务器
	go chatServer.Start()

	go func() {
		if err := engine.Run(fmt.Sprintf("%s:%d", host, port)); err != nil {
			zap.L().Fatal("server running fault")
			return
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	chatServer.Close()
	zap.L().Info("服务器已关闭")
}
```

**关键初始化顺序**：
1. 配置 → 2. 日志 → 3. 数据库 → 4. Redis → 5. JWT → 6. **Service 层** → 7. ChatServer → 8. Handlers → 9. SMS → 10. HTTP Server → 11. 启动

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
