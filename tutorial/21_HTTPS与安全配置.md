# 21. HTTPS 与安全配置

> 本教程将介绍如何为 HTTP 和 WebSocket 配置 TLS 加密，以及常见的安全防护措施。

---

## 📌 学习目标

- 生成自签名证书(开发用)
- 配置 Gin 开启 HTTPS/WSS
- 理解依赖注入与函数式服务器初始化
- 理解自定义 Logger 中间件
- 掌握 Nginx 反向代理配置
- 掌握安全防护最佳实践

---

## 1. 生成证书

在开发环境，可以生成自签名证书：

```bash
# 生成私钥
openssl genrsa -out server.key 2048

# 生成公钥/证书
openssl req -new -x509 -key server.key -out server.crt -days 365
```

**提示**：
- 不要提交到 Git（添加到 `.gitignore`）
- 生产环境使用 Let's Encrypt 或购买商业证书

---

## 2. HTTPS 服务器配置

### 2.1 internal/https_server/https_server.go

> **重要变更**：
> - `Init` 函数接收 `*handler.Handlers` 参数
> - 返回 `*gin.Engine` 实例，不再使用全局变量
> - 使用 `router.NewRouter(handlers)` 注入依赖

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
	// 使用 gin.New() 以便完全控制中间件
	engine := gin.New()

	// 使用自定义的 zap logger 中间件
	engine.Use(logger.GinLogger())
	engine.Use(logger.GinRecovery(true))

	// CORS 配置
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"*"} // 生产环境请指定具体域名
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	engine.Use(cors.New(corsConfig))

	// TLS 中间件（可选，生产环境通常由 Nginx 处理）
	// engine.Use(middleware.TlsHandler(config.GetConfig().MainConfig.Host, config.GetConfig().MainConfig.Port))

	// 静态资源映射
	engine.Static("/static/avatars", config.GetConfig().StaticAvatarPath)
	engine.Static("/static/files", config.GetConfig().StaticFilePath)

	// 创建路由管理器并注册所有业务路由
	rt := router.NewRouter(handlers)
	rt.RegisterRoutes(engine)

	return engine
}
```

**要点说明**：
- `gin.New()` 创建空白引擎，不含默认中间件
- 自定义 `logger.GinLogger()` 使用 Zap 记录结构化日志
- `logger.GinRecovery(true)` 捕获 panic 并记录堆栈
- 返回 `*gin.Engine` 供 main 函数使用

---

## 3. 主程序启动

### 3.1 HTTPS 模式启动

```go
package main

import (
	"fmt"
	"kama_chat_server/internal/config"
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/https_server"
	"go.uber.org/zap"
)

func main() {
	conf := config.GetConfig()
	
	// 初始化 Handlers（依赖注入）
	handlers := handler.NewHandlers(/* 注入依赖 */)
	
	// 初始化 HTTP 服务器
	engine := https_server.Init(handlers)

	addr := fmt.Sprintf("%s:%d", conf.MainConfig.Host, conf.MainConfig.Port)
	zap.L().Info("Starting HTTPS server on " + addr)

	// HTTPS 模式
	if err := engine.RunTLS(addr, "server.crt", "server.key"); err != nil {
		zap.L().Fatal("Server failed to start", zap.Error(err))
	}
}
```

### 3.2 HTTP 模式启动（Nginx 负责 SSL）

```go
// 使用普通 HTTP 启动
if err := engine.Run(":8000"); err != nil {
	zap.L().Fatal("Server failed to start", zap.Error(err))
}
```

---

## 4. Nginx 反向代理 (推荐)

在生产环境，通常由 Nginx 处理 SSL 卸载：

### 4.1 架构

```
用户浏览器 (HTTPS)
   ↓
Nginx (SSL 卸载)
   ↓ HTTP
Go 应用 (8000端口)
```

### 4.2 nginx.conf

```nginx
# HTTP 重定向到 HTTPS
server {
    listen 80;
    server_name chat.example.com;
    return 301 https://$server_name$request_uri;
}

# HTTPS 配置
server {
    listen 443 ssl http2;
    server_name chat.example.com;

    ssl_certificate /etc/nginx/cert/chat.pem;
    ssl_certificate_key /etc/nginx/cert/chat.key;
    ssl_protocols TLSv1.2 TLSv1.3;

    # HTTP 请求代理
    location / {
        proxy_pass http://localhost:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    # WebSocket 代理
    location /ws {
        proxy_pass http://localhost:8000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 600s;
    }

    # 静态文件 (可选优化)
    location /static/ {
        alias /path/to/kama_chat_server/static/;
        expires 30d;
    }
}
```

---

## 5. 安全防护 Checklist

| 风险 | 防护措施 | 实现位置 |
|-----|---------|---------|
| SQL 注入 | GORM 参数化查询 | `internal/dao/mysql/` |
| XSS 攻击 | JSON 响应自动转义 | `internal/handler/` |
| 密码泄露 | bcrypt 哈希 + BeforeSave Hook | `internal/model/user_info.go` |
| 暴力破解 | 验证码 1 分钟有效 + 失败回滚 | `internal/infrastructure/sms/` |
| CORS 滥用 | 生产环境指定 AllowOrigins | `internal/https_server/` |
| 文件上传 | Magic Bytes 校验 + 唯一文件名 | `internal/service/message/` |
| WebSocket 劫持 | 使用 WSS + JWT 验证 | `internal/service/chat/` |

---

## 6. CORS 安全配置

**开发环境**：
```go
corsConfig.AllowOrigins = []string{"*"}
```

**生产环境**：
```go
corsConfig.AllowOrigins = []string{
    "https://chat.example.com",
    "https://www.example.com",
}
corsConfig.AllowCredentials = true
```

---

## 7. 测试 HTTPS 连接

### 7.1 HTTP API

```bash
# 自签名证书需要 -k 跳过验证
curl -k https://localhost:8000/user/login \
  -H "Content-Type: application/json" \
  -d '{"telephone": "13800138000", "password": "123456"}'
```

### 7.2 WebSocket

```javascript
// 开发环境
let ws = new WebSocket("wss://localhost:8000/ws?client_id=U123456");

ws.onopen = () => console.log("✅ WSS 连接成功");
ws.onmessage = (evt) => console.log("📩 收到消息:", evt.data);
ws.onerror = (err) => console.error("❌ 连接错误:", err);
```

---

## 8. 常见问题排查

### 8.1 浏览器显示 "不安全的连接"

**解决**：
- 开发环境：点击「继续访问」
- 生产环境：使用 Let's Encrypt

```bash
sudo certbot --nginx -d chat.example.com
```

### 8.2 WebSocket 连接失败

**检查**：
1. 使用 `wss://` 而非 `ws://`
2. Nginx WebSocket 代理配置正确
3. 防火墙开放 443 端口

### 8.3 CORS 错误

**解决**：在 `https_server.go` 添加前端域名：

```go
corsConfig.AllowOrigins = []string{"https://chat.example.com"}
```

---

## ✅ 本节完成

你已经完成了：
- [x] 依赖注入的服务器初始化
- [x] 自定义 Logger 中间件配置
- [x] CORS 跨域配置
- [x] HTTPS/WSS 启动方式
- [x] Nginx 反向代理配置
- [x] 安全防护最佳实践

---

## 📚 项目教程完结

恭喜！你已经完成了 **KamaChat** 项目的全部核心教程。你可以：
- 继续完善音视频通话功能
- 添加消息已读/未读状态
- 实现消息撤回和引用
- 部署到生产环境
