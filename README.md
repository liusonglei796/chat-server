# KamaChat Server

<p align="center">
   <img src="https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version">
   <img src="https://img.shields.io/badge/Gin-v1.11.0-00ADD8?style=for-the-badge&logo=gin&logoColor=white" alt="Gin Framework">
  <img src="https://img.shields.io/badge/MySQL-8.0-4479A1?style=for-the-badge&logo=mysql&logoColor=white" alt="MySQL">
   <img src="https://img.shields.io/badge/Redis-v9-DC382D?style=for-the-badge&logo=redis&logoColor=white" alt="Redis">
  <img src="https://img.shields.io/badge/Kafka-Supported-231F20?style=for-the-badge&logo=apachekafka&logoColor=white" alt="Kafka">
  <img src="https://img.shields.io/badge/License-GPL--3.0-blue?style=for-the-badge" alt="License">
</p>

KamaChat Server 是一个基于 Go 语言开发的高性能即时通讯服务端，支持单聊、群聊、WebSocket 实时通信、Kafka 消息队列，以及 AI 智能回复、群聊总结、多语言翻译等功能。

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
- 🤖 **AI 智能回复建议** - 基于上下文生成 3 条可选回复
- 📋 **AI 群聊总结** - 自动提炼群聊要点、待办与决策（支持自定义时间范围和消息数量）
- 🌍 **AI 多语言翻译** - 支持自动识别源语言并翻译到目标语言
- 🛡️ **IDOR 防护** - 基于用户会话的 IDOR 安全检查

## 🏗️ 项目架构

```
kama_chat_server/
├── cmd/
│   └── kama_chat_server/         # 应用入口
│       └── main.go
├── configs/
│   └── config.toml               # 配置文件
├── docs/                         # 文档目录
│   ├── architecture_optimization_plan.md
│   ├── cache_strategy.md
│   ├── plans/                   # 开发计划
│   ├── security/                # 安全相关
│   └── tutorial/                 # 教程文档
├── internal/
│   ├── config/                   # 配置加载
│   ├── dao/                      # 数据访问层
│   │   ├── mysql/                # MySQL Repository
│   │   └── redis/                # Redis 缓存
│   │       └── cache/            # 缓存工具（singleflight）
│   ├── dto/                      # 数据传输对象
│   │   ├── request/              # 请求 DTO
│   │   └── respond/              # 响应 DTO
│   ├── gateway/                  # 网关层
│   │   └── websocket/            # WebSocket 实现
│   ├── handler/                  # HTTP 处理器
│   ├── https_server/             # HTTPS 服务器配置
│   ├── infrastructure/           # 基础设施层
│   │   ├── ai/                   # AI 客户端（Eino SDK）
│   │   ├── logger/               # 日志组件 (Zap)
│   │   ├── middleware/           # 中间件
│   │   ├── mq/                   # 消息队列 (Kafka)
│   │   ├── sms/                  # 短信服务
│   │   └── snowflake/            # 雪花算法 ID 生成
│   ├── model/                    # 数据模型
│   ├── router/                   # 路由定义
│   └── service/                  # 业务逻辑层
│       ├── ai/                   # AI 业务
│       ├── apply/                # 申请业务
│       ├── auth/                 # 认证业务
│       ├── friendship/          # 好友业务
│       ├── group/                # 群组业务
│       ├── message/              # 消息业务
│       ├── session/              # 会话业务
│       ├── user/                 # 用户业务
│       └── admin/                # 后台管理
├── migrations/                   # 数据库迁移脚本
├── pkg/
│   ├── aes/                      # AES 加密工具
│   ├── constants/                # 常量定义
│   │   ├── ai.go                 # AI 常量
│   │   ├── cache_key.go          # Redis Key 常量
│   │   └── constants.go          # 通用常量
│   ├── enum/                     # 枚举定义
│   │   ├── apply/
│   │   ├── friendship/
│   │   ├── group/
│   │   ├── message/
│   │   └── user/
│   ├── errorx/                   # 错误处理
│   ├── jwt/                      # JWT 工具
│   └── util/                     # 工具函数
├── test/                         # 测试文件
├── go.mod
├── go.sum
├── docker-compose.yml            # Docker 编排
├── docker-compose.local.yml      # 本地开发 Docker 编排
├── Dockerfile
└── LICENSE                       # GPL-3.0 许可证
```

## 🛠️ 技术栈

| 组件 | 技术 |
|------|------|
| **语言** | Go 1.26 |
| **Web 框架** | Gin v1.11 |
| **ORM** | GORM v1.31 |
| **数据库** | MySQL 8.0 |
| **缓存** | Redis v9 |
| **消息队列** | Kafka (可选) |
| **WebSocket** | Gorilla WebSocket |
| **日志** | Zap + Lumberjack |
| **认证** | JWT (golang-jwt/jwt) |
| **短信服务** | 阿里云 SMS |
| **配置管理** | TOML |
| **AI 编排** | ByteDance Eino |
| **AI 模型接入** | ModelScope OpenAI-Compatible API (Kimi K2.5) |
| **容器化** | Docker & Docker Compose |

## 🚀 快速开始

### 前置要求

- Go 1.26+
- MySQL 8.0+
- Redis 6.0+
- Kafka (可选，用于分布式消息处理)
- Docker & Docker Compose (可选，用于容器化部署)

### 本地开发

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
   hostPort = "127.0.0.1:9092"
   ```

4. **运行数据库迁移**

   确保 MySQL 数据库 `kama_chat` 已创建，并执行 `migrations/` 目录下的迁移脚本。

5. **启动服务**
   ```bash
   go run cmd/kama_chat_server/main.go
   ```

   服务将在 `http://0.0.0.0:8000` 启动。

### Docker 部署

#### 使用 Docker Compose (推荐)

```bash
# 本地开发环境（包含 MySQL、Redis、Kafka）
docker compose -f docker-compose.local.yml up -d

# 生产环境
docker compose -f docker-compose.yml up -d
```

#### 手动构建 Docker 镜像

```bash
# 构建镜像
docker build -t kamachat:latest .

# 运行容器
docker run -d \
  --name kamachat \
  -p 8000:8000 \
  -v ./configs/config.toml:/app/configs/config.toml \
  kamachat:latest
```

#### 查看日志

```bash
# Docker Compose 日志
docker compose logs -f kamachat

# 单个容器日志
docker logs -f kamachat
```

更多部署详情，请查看 [PRODUCTION_DEPLOYMENT.md](PRODUCTION_DEPLOYMENT.md) 和 [NGINX_GATEWAY_SETUP.md](NGINX_GATEWAY_SETUP.md)。

## 📡 API 模块

| 模块 | 路由前缀 | 说明 |
|------|---------|------|
| 认证模块 | `/auth` | 注册、登录、短信验证码、Token 刷新 |
| 用户模块 | `/user` | 用户信息管理 |
| 好友模块 | `/friends` | 好友关系、好友申请 |
| 群组模块 | `/groups` | 群组创建、成员管理、入群申请 |
| 会话模块 | `/sessions` | 会话列表、会话管理、会话置顶 |
| 消息模块 | `/messages` | 消息记录、撤回 |
| 上传模块 | `/upload` | 头像、文件上传 |
| AI 模块 | `/ai` | 智能回复建议、群聊总结、翻译 |
| WebSocket | `/ws` | 实时通信 |

## 🔧 配置说明

### 消息模式

KamaChat 目前使用 **Kafka 分布式消息队列模式** 作为唯一的内部消息总线：

- 支持高并发和水平扩展
- 提升了服务架构的扩展性
- 强解耦生产者与消费者

### JWT 配置

```toml
[jwtConfig]
secret = "your-super-secret-key"
accessTokenExpiry = 15      # Access Token 有效期（分钟）
refreshTokenExpiry = 168    # Refresh Token 有效期（小时）
```

### AI 配置（ModelScope + Kimi K2.5）

```toml
[modelScopeConfig]
apiKey = "" # 建议通过环境变量注入，不要写死在仓库
baseUrl = "https://api-inference.modelscope.cn/v1"
model = "moonshotai/Kimi-K2.5"
```

推荐环境变量：

- `MODELSCOPE_API_KEY`
- `MODELSCOPE_BASE_URL`
- `MODELSCOPE_MODEL`

PowerShell 示例：

```powershell
$env:MODELSCOPE_API_KEY="ms-你的Key"
$env:MODELSCOPE_MODEL="moonshotai/Kimi-K2.5"
go run cmd/kama_chat_server/main.go
```

## 📝 开发指南

### 目录结构规范

- `internal/handler/` - HTTP 请求处理，参数校验和响应格式化
- `internal/service/` - 业务逻辑实现，事务管理
- `internal/dao/` - 数据库访问，Repository 模式
- `internal/dto/` - 请求和响应的数据结构定义
- `internal/model/` - 数据库模型定义
- `docs/` - 项目文档与开发计划
- `migrations/` - 数据库迁移脚本

### 返回值约定

Service 层返回值约定：
- `ret = 0` - 服务调用成功
- `ret = -1` - 系统错误 (HTTP 500)
- `ret = -2` - 业务错误 (HTTP 400)

### 常量定义规范

- 通用常量：`pkg/constants/constants.go`
- Redis Key：`pkg/constants/cache_key.go`
- AI 常量：`pkg/constants/ai.go`
- 枚举定义：`pkg/enum/<模块>/<类型>/`

### AI 群聊总结参数说明

| 参数 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| group_id | string | 群组 ID（必须以 G 开头） | - |
| hours | int | 总结过去多少小时的消息 | 24 |
| limit | int | 最多拉取多少条消息 | 200（最大 500） |
| style | string | 总结风格（brief/comprehensive） | brief |

示例请求：
```bash
curl -X POST /ai/group-summary \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"group_id": "G123456", "hours": 24, "limit": 200}'
```

## 📚 相关文档

- [生产部署指南](PRODUCTION_DEPLOYMENT.md) - 生产环境配置与部署流程
- [Nginx 网关配置](NGINX_GATEWAY_SETUP.md) - 反向代理与网关设置
- [架构优化计划](docs/architecture_optimization_plan.md) - 系统架构演进方向
- [缓存策略](docs/cache_strategy.md) - Redis 缓存策略详解

## 📄 License

本项目采用 [GPL-3.0 License](LICENSE) 开源许可证。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

---

<p align="center">Made with ❤️ by KamaChat Team</p>
