# Phase 3: 配置管理强化方案

## 优化目标
防止无效配置启动，确保关键配置项完整

## 当前问题

### 1. 配置加载错误被忽略
**位置**: `internal/config/config.go:126`
```go
func GetConfig() *Config {
    if config == nil {
        config = new(Config)
        _ = LoadConfig() // 忽略加载错误，使用默认值
    }
    return config
}
```
**问题**: 配置文件不存在或格式错误时，使用零值配置继续启动
**风险**: MySQL 密码为空也能启动，导致运行时错误

### 2. 无配置验证
**问题**: 启动时不验证配置项有效性
**风险**: 
- JWT 密钥为空，所有 Token 可伪造
- 数据库连接信息缺失，启动后崩溃
- Redis 未配置，缓存功能异常

### 3. 默认值不合理
**问题**: 某些配置零值不合理
- Port = 0（无法监听）
- JWT Secret = ""（安全隐患）

---

## 优化方案

### 1. 添加配置验证函数

**实现文件**: `internal/config/validator.go` (新建)

```go
package config

import (
    "fmt"
    "errors"
)

// ValidationError 配置验证错误
type ValidationError struct {
    Field   string
    Message string
}

func (e ValidationError) Error() string {
    return fmt.Sprintf("配置项 [%s] 验证失败: %s", e.Field, e.Message)
}

// Validate 验证配置是否有效
// 返回错误列表，如果全部有效返回 nil
func (c *Config) Validate() []error {
    var errs []error
    
    // 验证主配置
    if err := c.validateMainConfig(); err != nil {
        errs = append(errs, err...)
    }
    
    // 验证 MySQL 配置
    if err := c.validateMySQLConfig(); err != nil {
        errs = append(errs, err...)
    }
    
    // 验证 Redis 配置
    if err := c.validateRedisConfig(); err != nil {
        errs = append(errs, err...)
    }
    
    // 验证 JWT 配置
    if err := c.validateJWTConfig(); err != nil {
        errs = append(errs, err...)
    }
    
    // 验证日志配置
    if err := c.validateLogConfig(); err != nil {
        errs = append(errs, err...)
    }
    
    // 验证雪花算法配置
    if err := c.validateSnowflakeConfig(); err != nil {
        errs = append(errs, err...)
    }
    
    if len(errs) > 0 {
        return errs
    }
    return nil
}

func (c *Config) validateMainConfig() []error {
    var errs []error
    
    if c.MainConfig.Host == "" {
        c.MainConfig.Host = "0.0.0.0" // 设置默认值
    }
    
    if c.MainConfig.Port <= 0 || c.MainConfig.Port > 65535 {
        errs = append(errs, ValidationError{
            Field:   "mainConfig.port",
            Message: "端口号必须在 1-65535 之间",
        })
    }
    
    if c.MainConfig.AppName == "" {
        c.MainConfig.AppName = "KamaChat" // 设置默认值
    }
    
    return errs
}

func (c *Config) validateMySQLConfig() []error {
    var errs []error
    
    if c.MysqlConfig.Host == "" {
        errs = append(errs, ValidationError{
            Field:   "mysqlConfig.host",
            Message: "MySQL 主机地址不能为空",
        })
    }
    
    if c.MysqlConfig.Port <= 0 || c.MysqlConfig.Port > 65535 {
        errs = append(errs, ValidationError{
            Field:   "mysqlConfig.port",
            Message: "MySQL 端口号必须在 1-65535 之间",
        })
    }
    
    if c.MysqlConfig.User == "" {
        errs = append(errs, ValidationError{
            Field:   "mysqlConfig.user",
            Message: "MySQL 用户名不能为空",
        })
    }
    
    // 密码可以为空（某些测试环境）
    
    if c.MysqlConfig.DatabaseName == "" {
        errs = append(errs, ValidationError{
            Field:   "mysqlConfig.databaseName",
            Message: "MySQL 数据库名称不能为空",
        })
    }
    
    return errs
}

func (c *Config) validateRedisConfig() []error {
    var errs []error
    
    if c.RedisConfig.Host == "" {
        errs = append(errs, ValidationError{
            Field:   "redisConfig.host",
            Message: "Redis 主机地址不能为空",
        })
    }
    
    if c.RedisConfig.Port <= 0 || c.RedisConfig.Port > 65535 {
        errs = append(errs, ValidationError{
            Field:   "redisConfig.port",
            Message: "Redis 端口号必须在 1-65535 之间",
        })
    }
    
    if c.RedisConfig.Db < 0 {
        errs = append(errs, ValidationError{
            Field:   "redisConfig.db",
            Message: "Redis 数据库编号不能为负数",
        })
    }
    
    return errs
}

func (c *Config) validateJWTConfig() []error {
    var errs []error
    
    if c.JWTConfig.Secret == "" {
        errs = append(errs, ValidationError{
            Field:   "jwtConfig.secret",
            Message: "JWT 密钥不能为空（建议至少32位随机字符串）",
        })
    }
    
    if len(c.JWTConfig.Secret) < 16 {
        errs = append(errs, ValidationError{
            Field:   "jwtConfig.secret",
            Message: "JWT 密钥太短（建议至少32位）",
        })
    }
    
    if c.JWTConfig.AccessTokenExpiry <= 0 {
        errs = append(errs, ValidationError{
            Field:   "jwtConfig.accessTokenExpiry",
            Message: "Access Token 有效期必须大于0",
        })
    }
    
    if c.JWTConfig.RefreshTokenExpiry <= 0 {
        errs = append(errs, ValidationError{
            Field:   "jwtConfig.refreshTokenExpiry",
            Message: "Refresh Token 有效期必须大于0",
        })
    }
    
    return errs
}

func (c *Config) validateLogConfig() []error {
    var errs []error
    
    if c.LogConfig.LogPath == "" {
        errs = append(errs, ValidationError{
            Field:   "logConfig.logPath",
            Message: "日志路径不能为空",
        })
    }
    
    if c.LogConfig.FileName == "" {
        errs = append(errs, ValidationError{
            Field:   "logConfig.fileName",
            Message: "日志文件名不能为空",
        })
    }
    
    validLevels := map[string]bool{
        "debug": true, "info": true, "warn": true, "error": true,
    }
    if !validLevels[c.LogConfig.Level] {
        errs = append(errs, ValidationError{
            Field:   "logConfig.level",
            Message: "日志级别必须是 debug/info/warn/error 之一",
        })
    }
    
    return errs
}

func (c *Config) validateSnowflakeConfig() []error {
    var errs []error
    
    if c.SnowflakeConfig.MachineID < 0 || c.SnowflakeConfig.MachineID > 1023 {
        errs = append(errs, ValidationError{
            Field:   "snowflakeConfig.machineId",
            Message: "雪花算法机器ID必须在 0-1023 之间",
        })
    }
    
    return errs
}
```

### 2. 修改配置加载逻辑

**修改文件**: `internal/config/config.go`

```go
// GetConfig 获取全局配置实例（单例模式）
// 首次调用时会自动加载配置文件并验证
// 如果配置无效，会 panic 阻止启动
func GetConfig() *Config {
    if config == nil {
        config = new(Config)
        
        // 加载配置
        if err := LoadConfig(); err != nil {
            log.Fatalf("加载配置文件失败: %v", err)
        }
        
        // 验证配置
        if errs := config.Validate(); errs != nil {
            log.Println("配置验证失败:")
            for _, err := range errs {
                log.Printf("  - %v\n", err)
            }
            log.Fatalln("请检查配置文件并修正上述错误")
        }
        
        log.Println("配置加载并验证成功")
    }
    return config
}
```

### 3. 添加 WebSocket 配置

**修改文件**: `internal/config/config.go`

```go
// WebSocketConfig WebSocket 配置
type WebSocketConfig struct {
    MaxConnections     int32         `toml:"maxConnections"`     // 最大连接数
    HeartbeatInterval  time.Duration `toml:"heartbeatInterval"`  // 心跳间隔（秒）
    HeartbeatTimeout   int           `toml:"heartbeatTimeout"`   // 心跳超时次数
    EnableAck          bool          `toml:"enableAck"`          // 是否启用消息确认
    AckTimeout         time.Duration `toml:"ackTimeout"`         // 确认超时（秒）
    MessageRateLimit   int           `toml:"messageRateLimit"`   // 消息速率限制（条/秒）
}

// Config 添加 WebSocketConfig 字段
type Config struct {
    // ... 原有字段
    WebSocketConfig `toml:"websocketConfig"` // WebSocket 配置
}

// 验证函数中添加
func (c *Config) validateWebSocketConfig() []error {
    var errs []error
    
    // 设置默认值
    if c.WebSocketConfig.MaxConnections <= 0 {
        c.WebSocketConfig.MaxConnections = 10000
    }
    
    if c.WebSocketConfig.HeartbeatInterval <= 0 {
        c.WebSocketConfig.HeartbeatInterval = 30
    }
    
    if c.WebSocketConfig.HeartbeatTimeout <= 0 {
        c.WebSocketConfig.HeartbeatTimeout = 3
    }
    
    if c.WebSocketConfig.AckTimeout <= 0 {
        c.WebSocketConfig.AckTimeout = 10
    }
    
    if c.WebSocketConfig.MessageRateLimit <= 0 {
        c.WebSocketConfig.MessageRateLimit = 10
    }
    
    // 验证值合理性
    if c.WebSocketConfig.MaxConnections > 100000 {
        errs = append(errs, ValidationError{
            Field:   "websocketConfig.maxConnections",
            Message: "最大连接数不能超过 100000",
        })
    }
    
    return errs
}
```

### 4. 配置文件模板更新

**修改文件**: `configs/config.toml`

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

[jwtConfig]
secret = "your-super-secret-key-must-be-at-least-32-characters-long"
accessTokenExpiry = 15      # 分钟
refreshTokenExpiry = 168    # 小时

[logConfig]
logPath = "./logs"
fileName = "kama_chat.log"
maxSize = 100      # MB
maxBackups = 5
maxAge = 30        # 天
level = "info"     # debug/info/warn/error

[snowflakeConfig]
machineId = 1      # 0-1023

# 新增 WebSocket 配置
[websocketConfig]
maxConnections = 10000          # 最大连接数
heartbeatInterval = 30          # 心跳间隔（秒）
heartbeatTimeout = 3            # 心跳超时次数
enableAck = true                # 启用消息确认
ackTimeout = 10                 # 确认超时（秒）
messageRateLimit = 10           # 消息速率限制（条/秒）

# 其他配置...
```

---

## 实施步骤

### Step 1: 创建配置验证器 (40分钟)
- 创建 `internal/config/validator.go`
- 实现各个配置项的验证函数
- 添加合理的默认值设置

### Step 2: 修改配置加载 (20分钟)
- 修改 `config.go:GetConfig()`
- 添加验证调用
- 配置无效时阻止启动

### Step 3: 添加 WebSocket 配置 (20分钟)
- 修改 `config.go`，添加 WebSocketConfig 结构体
- 添加验证函数
- 设置默认值

### Step 4: 更新配置文件模板 (10分钟)
- 更新 `configs/config.toml`
- 添加注释说明

### Step 5: 修改 main.go (10分钟)
- 确保配置加载在初始化之前
- 添加配置加载失败的处理

---

## 测试方案

### 1. 有效配置测试
```bash
# 使用完整正确的配置启动
go run cmd/kama_chat_server/main.go
# 应正常启动，日志输出"配置加载并验证成功"
```

### 2. 无效配置测试
```bash
# 测试空 JWT 密钥
cat > configs/config_test.toml << 'EOF'
[jwtConfig]
secret = ""
EOF

# 启动应失败，显示错误信息
```

### 3. 缺失配置测试
```bash
# 删除 MySQL 配置段
# 启动应失败，提示 MySQL 配置缺失
```

---

## 预期效果

- ✅ 配置无效时阻止启动，避免运行时错误
- ✅ 明确的错误提示，便于排查
- ✅ 合理的默认值，减少配置工作量
- ✅ WebSocket 参数可配置化

---

## 回滚方案

如果出现问题：
1. 恢复 `config.go`，移除验证逻辑
2. 删除 `validator.go`
3. 恢复 `config.toml`
4. 重启服务
