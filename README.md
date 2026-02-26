# KamaChat Server

<p align="center">
   <img src="https://img.shields.io/badge/Go-1.24-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version">
   <img src="https://img.shields.io/badge/Gin-v1.11.0-00ADD8?style=for-the-badge&logo=gin&logoColor=white" alt="Gin Framework">
  <img src="https://img.shields.io/badge/MySQL-8.0-4479A1?style=for-the-badge&logo=mysql&logoColor=white" alt="MySQL">
   <img src="https://img.shields.io/badge/Redis-v9-DC382D?style=for-the-badge&logo=redis&logoColor=white" alt="Redis">
  <img src="https://img.shields.io/badge/Kafka-Supported-231F20?style=for-the-badge&logo=apachekafka&logoColor=white" alt="Kafka">
  <img src="https://img.shields.io/badge/License-GPL--3.0-blue?style=for-the-badge" alt="License">
</p>

KamaChat Server 是一个基于 Go 语言开发的高性能即时通讯服务端，支持单聊、群聊、WebSocket 实时通信、Kafka 消息队列、AI 消息翻译等功能。

## ✨ 功能特性

- 🔐 **JWT 双 Token 认证** - 支持 Access Token + Refresh Token 双令牌机制，实现无感刷新
- 👤 **用户管理** - 用户注册、登录、短信验证码登录、个人信息管理
- 💬 **单聊与群聊** - 支持一对一私聊及群组聊天功能
- 🔌 **WebSocket 实时通信** - 基于 Gorilla WebSocket 实现实时消息推送
- 📮 **消息模式可选** - 支持 Channel 模式和 Kafka 分布式消息队列模式
- 👥 **好友与群组管理** - 好友申请、群组创建、成员管理等
- 📱 **短信服务** - 集成阿里云短信服务，支持验证码发送
- 📁 **文件上传** - 支持头像、文件等静态资源上传与管理
- 🔒 **HTTPS 支持** - 支持安全的 HTTPS 通信
- 🎥 **音视频信令** - 支持音视频通话信令转发
- ↩️ **消息撤回** - 支持在规定时间内撤回已发送的消息
- 📌 **会话置顶** - 支持将重要会话置顶显示
- 🔇 **群成员禁言** - 群主/管理员可对群成员进行禁言操作
- 📝 **好友备注** - 支持设置好友备注名，方便管理联系人
- 🌍 **AI 消息翻译** - 基于 Eino + ModelScope，支持翻译开关与目标语言自定义

## 🏗️ 项目架构

```
kama_chat_server/
├── cmd/
│   └── kama_chat_server/         # 应用入口
│       └── main.go
├── configs/
│   └── config.toml               # 配置文件
├── docs/                         # 文档目录
├── internal/
│   ├── config/                   # 配置加载
│   ├── dao/                      # 数据访问层 (MySQL + Redis)
│   ├── dto/                      # 数据传输对象 (Request & Response)
│   ├── gateway/                  # 网关层 (WebSocket)
│   │   └── websocket/
│   ├── handler/                  # HTTP 处理器
│   ├── https_server/             # HTTPS 服务器配置
│   ├── infrastructure/           # 基础设施层
│   │   ├── logger/               # 日志组件 (Zap)
│   │   ├── middleware/           # 中间件 (JWT, CORS 等)
│   │   ├── mq/                   # 消息队列 (Kafka)
│   │   └── sms/                  # 短信服务
│   ├── model/                    # 数据模型
│   ├── router/                   # 路由定义
│   ├── service/                  # 业务逻辑层
│   └── tutorial/                 # 教程文档
├── migrations/                   # 数据库迁移脚本
├── pkg/
│   ├── aes/                      # AES 加密工具
│   ├── constants/                # 常量定义
│   ├── enum/                     # 枚举定义
│   ├── errorx/                   # 错误处理
│   └── util/                     # 工具函数
│       └── jwt/                  # JWT 工具
├── test/                         # 测试文件
├── go.mod
├── go.sum
└── LICENSE                       # GPL-3.0 许可证
```

## 🛠️ 技术栈

| 组件 | 技术 |
|------|------|
| **语言** | Go 1.24 |
| **Web 框架** | Gin v1.11 |
| **ORM** | GORM v1.25 |
| **数据库** | MySQL 8.0 |
| **缓存** | Redis v9 |
| **消息队列** | Kafka (可选) |
| **WebSocket** | Gorilla WebSocket |
| **日志** | Zap + Lumberjack |
| **认证** | JWT (golang-jwt/jwt) |
| **短信服务** | 阿里云 SMS |
| **AI 框架** | Eino |
| **LLM 接入** | ModelScope (OpenAI 兼容 API) |
| **配置管理** | TOML |

## 🚀 快速开始

### 前置要求

- Go 1.24+
- MySQL 8.0+
- Redis 6.0+
- Kafka (可选，用于分布式消息处理)

### 安装步骤

1. **克隆仓库**
   ```bash
   git clone git@github.com:liusonglei796/chat-server.git
   cd chat-server
   ```

2. **安装依赖**
   ```bash
   go mod download
   ```

3. **配置文件**

   编辑 `configs/config.toml`，配置数据库、Redis、短信服务等：

   ```toml
   [mainConfig]
   appName = "KamaChat"
   host = "0.0.0.0"
   port = 8000
   
   [mysqlConfig]
   host = "127.0.0.1"
   port = 3306
   user = "root"
   password = "your_password"
   databaseName = "kama_chat"
   
   [redisConfig]
   host = "127.0.0.1"
   port = 6379
   password = ""
   db = 0
   
   [kafkaConfig]
   messageMode = "channel"  # 或 "kafka"
   hostPort = "127.0.0.1:9092"

   [modelScopeConfig]
   apiKey = "your_modelscope_token"
   baseUrl = "https://api-inference.modelscope.cn/v1"
   model = "moonshotai/Kimi-K2.5"
   ```

4. **运行数据库迁移**

   确保 MySQL 数据库 `kama_chat` 已创建，并执行 `migrations/` 目录下的迁移脚本。

5. **启动服务**
   ```bash
   go run cmd/kama_chat_server/main.go
   ```

   服务将在 `http://0.0.0.0:8000` 启动。

## 📡 API 模块

| 模块 | 路由前缀 | 说明 |
|------|---------|------|
| 认证模块 | `/auth` | 注册、登录、短信验证码、Token 刷新 |
| 用户模块 | `/user` | 用户信息管理 |
| 好友模块 | `/friends` | 好友关系、好友申请 |
| 群组模块 | `/groups` | 群组创建、成员管理、入群申请 |
| 会话模块 | `/sessions` | 会话列表、会话管理、会话置顶 |
| 消息模块 | `/messages` | 消息记录、撤回、翻译 |
| 上传模块 | `/upload` | 头像、文件上传 |
| WebSocket | `/ws` | 实时通信 |

### 翻译接口示例

- 路径：`POST /messages/translate`
- 请求体：

```json
{
   "messageId": "M202602260001",
   "enableTranslate": true,
   "targetLang": "日语"
}
```

- 字段说明：
   - `messageId`：要翻译的消息 ID（必填）
   - `enableTranslate`：是否开启翻译（可选，不传默认 `true`）
   - `targetLang`：目标语言（可选，空值默认“英语”）

## 🔧 配置说明

### 消息模式

KamaChat 支持两种消息处理模式：

- **Channel 模式** (`messageMode = "channel"`)
  - 单机部署，使用 Go Channel 进行消息传递
  - 适合中小规模应用

- **Kafka 模式** (`messageMode = "kafka"`)
  - 分布式部署，使用 Kafka 消息队列
  - 支持高并发和水平扩展

### JWT 配置

```toml
[jwtConfig]
secret = "your-super-secret-key"
accessTokenExpiry = 15      # Access Token 有效期（分钟）
refreshTokenExpiry = 168    # Refresh Token 有效期（小时）
```

## 📝 开发指南

### 目录结构规范

- `internal/handler/` - HTTP 请求处理，参数校验和响应格式化
- `internal/service/` - 业务逻辑实现，事务管理
- `internal/dao/` - 数据库访问，Repository 模式
- `internal/dto/` - 请求和响应的数据结构定义
- `internal/model/` - 数据库模型定义

### 返回值约定

Service 层返回值约定：
- `ret = 0` - 服务调用成功
- `ret = -1` - 系统错误 (HTTP 500)
- `ret = -2` - 业务错误 (HTTP 400)

## 📄 License

本项目采用 [GPL-3.0 License](LICENSE) 开源许可证。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

---

<p align="center">Made with ❤️ by KamaChat Team</p>
