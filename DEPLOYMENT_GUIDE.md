# 🚀 KamaChat 全栈项目部署与服务器运维手册

这份文档整合了项目开发、容器化部署、域名解析、多项目管理及 SSL 安全配置的全流程指南。

---

## 一、 项目架构概览 (Full-Stack Overview)

本项目采用前后端分离架构，通过 Docker 进行全组件容器化，并利用 Nginx 作为唯一入口实现多项目并存。

*   **后端 (Go)**: 处理 API 业务逻辑、WebSocket 长连接、分布式 ID 生成 (Snowflake)。
*   **前端 (Vue 3)**: 提供 UI 交互、Pinia 状态管理、实时消息推送。
*   **中间件**: MySQL (持久化)、Redis (缓存)、Kafka (消息队列)。
*   **网关 (Nginx)**: 负责流量分发、子域名解析、SSL 证书卸载。

---

## 二、 前端开发与构建 (`/frontend`)

前端使用 Vue 3 (Composition API) + TypeScript 开发。

### 核心功能实现：
*   **鉴权系统**: `LoginView.vue` 和 `RegisterView.vue` 处理用户入驻。
*   **状态管理**: 使用 Pinia (`stores/chat.ts`) 统一管理会话列表、历史消息和 WebSocket 连接状态。
*   **实时通信**: 封装了 WebSocket 逻辑，支持自动接收推送消息并实时更新 UI。

### Docker 构建逻辑：
采用**多阶段构建**以减小镜像体积：
1.  **Stage 1 (Build)**: Node.js 编译生成 `dist` 静态文件。
2.  **Stage 2 (Production)**: 轻量级 Nginx 镜像托管 `dist`，并配置 `try_files` 支持 History 路由。

---

## 三、 阿里云短信 (Aliyun SMS) 集成

### 1. 云端准备
在阿里云控制台获取：`AccessKey ID`、`AccessKey Secret`、`SignName`（签名）、`TemplateCode`（模板 ID）。

### 2. 环境变量配置
在项目根目录创建 `.env` 文件（不要上传至 Git）：
```env
# 阿里云短信配置
SMS_ACCESS_KEY_ID=你的AccessKeyID
SMS_ACCESS_KEY_SECRET=你的AccessKeySecret
SMS_SIGN_NAME=你的签名
SMS_TEMPLATE_CODE=你的模版ID
```

---

## 四、 服务器多项目管理 (多子域名部署)

当你有一台服务器需要跑两个 Web 项目（如 `chat` 和 `blog`）时，遵循以下方案：

### 1. DNS 域名解析 (拆分子域名)
在域名服务商后台添加两条 **A 记录**，均指向服务器公网 IP：
*   主机记录 `chat` -> 对应 `chat.yourdomain.com`
*   主机记录 `blog` -> 对应 `blog.yourdomain.com`

### 2. 全局网关模式 (推荐方案)
**不建议**每个项目各搞一个 Nginx，而是使用 **“一个全局 Nginx”** 统一管理所有流量。

#### 核心配置参考 (`nginx.conf`)：
```nginx
http {
    # 流量转发给项目 A (聊天室)
    server {
        listen 80;
        server_name chat.yourdomain.com;
        location / { proxy_pass http://chat-frontend:80; }
        location /api { proxy_pass http://chat-app:8000; }
        location /ws {
            proxy_pass http://chat-app:8000;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
        }
    }

    # 流量转发给项目 B (如个人博客，假设其容器名为 blog-container 端口 3000)
    server {
        listen 80;
        server_name blog.yourdomain.com;
        location / {
            proxy_pass http://blog-container:3000; 
            proxy_set_header Host $host;
        }
    }
}
```

---

## 五、 SSL/TLS (HTTPS) 证书配置

### 方案 A：手动配置 (如果你已有 `.pem` 和 `.key` 文件)
1.  将证书文件挂载进 Nginx 容器。
2.  修改配置文件开启 443 端口并引用证书路径。

### 方案 B：可视化管理 (强烈推荐神器)
部署 **Nginx Proxy Manager (NPM)**：
*   **优势**: 零代码配置，在网页 UI 上填入域名即可完成转发。
*   **一键 SSL**: 集成 Let's Encrypt，网页点击即可申请并**自动续期**。

---

## 六、 部署 Checklist 与启动命令

### 1. 准备工作
*   [ ] 服务器已安装 Docker 和 Docker Compose。
*   [ ] 云平台安全组已放行 `80`, `443`, `3306`, `6379`, `9092` 端口。
*   [ ] 域名 A 记录已解析完成并生效。
*   [ ] `.env` 敏感信息已配置。

### 2. 启动项目
在根目录下执行：
```bash
# 构建并启动所有容器
docker-compose up --build -d

# 查看运行状态
docker-compose ps

# 查看实时日志 (排查问题)
docker logs -f chat-app
```

---

**💡 专家提示**: 
*   **容器网络**: 确保所有需要被 Nginx 访问的容器都加入了同一个 `docker network`（例如 `chat-network`）。
*   **数据持久化**: 务必检查 `docker-compose.yml` 中的 `volumes` 配置，确保数据库数据不会因为容器销毁而丢失。

--- 
*文档版本：1.0.0 | 维护者：Gemini CLI Agent*
