# KamaChat 生产环境部署指南

## 一、生产环境检查清单

### 1. 安全配置 🔒

#### JWT密钥更新 (必须)
```bash
# 当前配置 (开发环境默认值)
JWT_SECRET=kama-chat-super-secret-key-change-in-production

# ❌ 不要在生产环境使用此密钥
# ✅ 生成新的强密钥
openssl rand -base64 32
# 或
head -c 32 /dev/urandom | base64

# 更新 docker-compose.yml
environment:
  - JWT_SECRET=<生成的强密钥>
```

#### MySQL密码更新 (必须)
```bash
# 当前密码 (开发环境默认值)
MYSQL_PASSWORD=root123456
MYSQL_ROOT_PASSWORD=root123456

# ✅ 修改为强密码 (至少12字符)
MYSQL_PASSWORD=<strong_password_here>
MYSQL_ROOT_PASSWORD=<strong_password_here>
```

#### Redis密码配置 (推荐)
```bash
# 当前 (无密码)
REDIS_PASSWORD=

# ✅ 添加密码
docker-compose.yml:
  redis:
    command: redis-server --requirepass <redis_password>

environment:
  - REDIS_PASSWORD=<redis_password>
```

### 2. 应用配置 ⚙️

#### GIN调试模式 (必须关闭)
```bash
# 当前 (开发模式)
[GIN-debug] [WARNING] Running in "debug" mode

# ✅ 在docker-compose.yml中添加
environment:
  - GIN_MODE=release
  - GIN_GIN_GONIC=1
```

#### 日志级别调整
```bash
# 生产环境推荐配置
environment:
  - LOG_LEVEL=info  # 改为info，减少debug日志量
```

### 3. Nginx反向代理配置 (推荐)

```nginx
upstream kamachat_backend {
    server kamachat-app:8000;
}

server {
    listen 80;
    server_name api.yourdomain.com;
    
    # 重定向到HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name api.yourdomain.com;
    
    # SSL证书配置
    ssl_certificate /etc/ssl/certs/domain.crt;
    ssl_certificate_key /etc/ssl/private/domain.key;
    
    # SSL安全配置
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    
    # 反向代理设置
    location / {
        proxy_pass http://kamachat_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebSocket支持
        proxy_read_timeout 86400;
        proxy_send_timeout 86400;
    }
    
    # 限流配置
    limit_req_zone $binary_remote_addr zone=api_limit:10m rate=100r/s;
    limit_req zone=api_limit burst=200 nodelay;
}
```

### 4. 资源限制配置

```yaml
services:
  app:
    # 内存限制
    mem_limit: 2g
    memswap_limit: 2g
    
    # CPU限制
    cpus: '2.0'
    cpuset: '0,1'
    
    # 重启策略
    restart_policy:
      condition: on-failure
      delay: 5s
      max_attempts: 5
    
    # 健康检查
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
  
  mysql:
    mem_limit: 1g
    memswap_limit: 1g
    
  redis:
    mem_limit: 512m
    memswap_limit: 512m
```

### 5. 数据库备份策略

#### MySQL备份
```bash
# 使用docker-compose备份
docker exec kamachat-mysql mysqldump -u root -proot123456 \
  kama_chat > backup_$(date +%Y%m%d_%H%M%S).sql

# 或添加自动备份任务到crontab
0 2 * * * docker exec kamachat-mysql mysqldump -u root -p$MYSQL_PASSWORD kama_chat | gzip > /backup/kama_chat_$(date +\%Y\%m\%d).sql.gz
```

#### Redis RDB备份
```bash
# 手动备份
docker exec kamachat-redis redis-cli BGSAVE

# 自动备份（修改docker-compose.yml）
redis:
  command: redis-server --appendonly yes --appendfilename "appendonly.aof"
```

---

## 二、部署流程

### 1. 环境准备
```bash
# 克隆项目
git clone <repo-url>
cd kama_chat_server

# 创建 .env 文件
cat > .env << EOF
MODELSCOPE_API_KEY=ms-efe04ce3-afcf-43e4-b67e-f57937a9aa5b
SMS_API_KEY=your_sms_api_key  # 配置真实短信服务
EOF

# 配置权限
chmod 600 .env
```

### 2. docker-compose.yml 生产配置

```yaml
version: '3.8'

services:
  mysql:
    image: mysql:8.0
    container_name: kamachat-mysql
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD}
      MYSQL_DATABASE: kama_chat
    volumes:
      - mysql_data:/var/lib/mysql
      - ./init.sql:/docker-entrypoint-initdb.d/init.sql
    ports:
      - "127.0.0.1:3306:3306"  # 仅允许本地连接
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    container_name: kamachat-redis
    command: redis-server --requirepass ${REDIS_PASSWORD} --maxmemory 512mb --maxmemory-policy allkeys-lru
    volumes:
      - redis_data:/data
    ports:
      - "127.0.0.1:6379:6379"  # 仅允许本地连接
    healthcheck:
      test: ["CMD", "redis-cli", "--raw", "incr", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  kafka:
    image: apache/kafka:4.1.1
    container_name: kamachat-kafka
    environment:
      KAFKA_NODE_ID: 1
      KAFKA_PROCESS_ROLES: broker,controller
      KAFKA_LISTENERS: PLAINTEXT://:9092,CONTROLLER://:9093
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:9092
      KAFKA_CONTROLLER_LISTENER_NAMES: CONTROLLER
      KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
      KAFKA_CONTROLLER_QUORUM_VOTERS: 1@kafka:9093
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
      KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR: 1
      KAFKA_TRANSACTION_STATE_LOG_MIN_ISR: 1
    ports:
      - "127.0.0.1:9092:9092"  # 仅允许本地连接
    healthcheck:
      test: ["CMD-SHELL", "/opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092"]
      interval: 30s
      timeout: 10s
      retries: 5
    restart: unless-stopped

  app:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: kamachat-app
    env_file: .env
    environment:
      - MYSQL_HOST=mysql
      - MYSQL_PASSWORD=${MYSQL_PASSWORD}
      - REDIS_HOST=redis
      - REDIS_PASSWORD=${REDIS_PASSWORD}
      - JWT_SECRET=${JWT_SECRET}
      - KAFKA_HOST_PORT=kafka:9092
      - GIN_MODE=release
      - LOG_LEVEL=info
    ports:
      - "127.0.0.1:8000:8000"  # 建议仅本地监听，通过Nginx代理
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy
      kafka:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/health || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    restart: unless-stopped
    mem_limit: 2g
    cpus: '2.0'
    networks:
      - kamachat-network

volumes:
  mysql_data:
  redis_data:

networks:
  kamachat-network:
    driver: bridge
```

### 3. 启动命令
```bash
# 使用生产环境配置启动
docker compose -f docker-compose.yml up -d

# 验证所有服务
docker compose ps

# 查看日志
docker compose logs -f

# 检查健康状态
docker exec kamachat-app curl http://localhost:8000/health
```

---

## 三、监控和日志

### 1. Prometheus监控配置

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'kamachat'
    static_configs:
      - targets: ['localhost:8000']
```

### 2. ELK日志收集

```dockerfile
# filebeat配置
filebeat.inputs:
- type: container
  paths:
    - '/var/lib/docker/containers/*/*.log'

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
```

### 3. Grafana仪表板

监控指标：
- HTTP请求数/错误率
- 数据库连接池状态
- Redis内存使用率
- Kafka生产/消费延迟

---

## 四、故障恢复

### 1. 数据库恢复
```bash
# 从备份恢复
docker exec -i kamachat-mysql mysql -u root -proot123456 kama_chat < backup.sql
```

### 2. Redis清理
```bash
# 清空Redis缓存 (谨慎操作)
docker exec kamachat-redis redis-cli FLUSHALL
```

### 3. 应用重启
```bash
# 平滑重启
docker compose restart app

# 强制重启
docker compose down && docker compose up -d
```

---

## 五、安全加固

### 1. 防火墙规则
```bash
# 仅允许HTTPS访问
ufw allow 443/tcp
ufw allow 80/tcp  # HTTP重定向
ufw deny 3306/tcp  # 禁止MySQL外网访问
ufw deny 6379/tcp  # 禁止Redis外网访问
```

### 2. SSL/TLS证书
```bash
# 使用Let's Encrypt自动化证书
certbot certonly --standalone -d api.yourdomain.com
```

### 3. API速率限制
```nginx
# 在Nginx中配置
limit_req_zone $binary_remote_addr zone=general:10m rate=100r/s;
limit_req_zone $binary_remote_addr zone=auth:10m rate=5r/m;  # 认证端点

location /auth/ {
    limit_req zone=auth burst=10 nodelay;
    proxy_pass http://backend;
}
```

---

## 六、性能优化建议

### 1. 数据库索引
```sql
-- 常用查询优化
CREATE INDEX idx_user_telephone ON user_info(telephone);
CREATE INDEX idx_message_created_at ON message(created_at);
CREATE INDEX idx_session_user ON session(send_id, receive_id);
```

### 2. Redis优化
```bash
# 增大内存限制
redis:
  command: redis-server --maxmemory 2gb --maxmemory-policy allkeys-lru
```

### 3. Kafka优化
```bash
# 增加副本因子
kafka:
  environment:
    - KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=3
    - KAFKA_DEFAULT_REPLICATION_FACTOR=3
```

---

## 七、定期维护任务

| 任务 | 频率 | 命令 |
|------|------|------|
| 数据库备份 | 每天 | `mysqldump ...` |
| 日志清理 | 每周 | `docker logs --follow` |
| 安全更新 | 每月 | `docker pull && rebuild` |
| 性能分析 | 每周 | `docker stats` |
| 磁盘检查 | 每周 | `df -h` |

---

## 八、快速问题排查

### 应用无法启动
```bash
docker logs kamachat-app --tail 100
# 检查是否有依赖服务未启动
docker compose ps
```

### 数据库连接失败
```bash
docker exec kamachat-app mysql -h mysql -u root -p$MYSQL_PASSWORD -e "SELECT 1"
```

### Redis连接超时
```bash
docker exec kamachat-redis redis-cli ping
```

### Kafka消息堆积
```bash
docker exec kamachat-kafka /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server localhost:9092 --list
```

---

**文档版本:** v1.0  
**最后更新:** 2026-03-03  
**维护者:** DevOps Team
