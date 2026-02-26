// Package config 提供应用程序的配置加载和管理功能
// 使用 TOML 格式的配置文件，支持多路径查找
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/BurntSushi/toml" // TOML 配置文件解析库
)

// MainConfig 主配置，包含应用基本信息
type MainConfig struct {
	AppName string `toml:"appName"` // 应用名称，用于日志标识等
	Host    string `toml:"host"`    // 服务器监听地址，如 "0.0.0.0"
	Port    int    `toml:"port"`    // 服务器监听端口，如 8000
}

// MysqlConfig MySQL 数据库连接配置
type MysqlConfig struct {
	Host         string `toml:"host"`         // MySQL 服务器地址
	Port         int    `toml:"port"`         // MySQL 端口，默认 3306
	User         string `toml:"user"`         // 数据库用户名
	Password     string `toml:"password"`     // 数据库密码
	DatabaseName string `toml:"databaseName"` // 数据库名称
}

// RedisConfig Redis 连接配置
type RedisConfig struct {
	Host     string `toml:"host"`     // Redis 服务器地址
	Port     int    `toml:"port"`     // Redis 端口，默认 6379
	Password string `toml:"password"` // Redis 密码，无密码留空
	Db       int    `toml:"db"`       // Redis 数据库编号，默认 0
}

// AuthCodeConfig 短信验证码服务配置（阿里云 SMS）
type AuthCodeConfig struct {
	AccessKeyID     string `toml:"accessKeyID"`     // 阿里云 AccessKey ID
	AccessKeySecret string `toml:"accessKeySecret"` // 阿里云 AccessKey Secret
	SignName        string `toml:"signName"`        // 短信签名名称
	TemplateCode    string `toml:"templateCode"`    // 短信模板 Code
}

// LogConfig 日志配置，使用 lumberjack 进行日志轮转
type LogConfig struct {
	LogPath    string `toml:"logPath"`    // 日志文件存储目录
	FileName   string `toml:"fileName"`   // 日志文件名
	MaxSize    int    `toml:"maxSize"`    // 单个日志文件最大大小（MB）
	MaxBackups int    `toml:"maxBackups"` // 保留旧日志文件的最大个数
	MaxAge     int    `toml:"maxAge"`     // 保留旧日志文件的最大天数
	Level      string `toml:"level"`      // 日志级别：debug, info, warn, error
}

// KafkaConfig Kafka 消息队列配置
type KafkaConfig struct {
	HostPort    string        `toml:"hostPort"`    // Kafka 服务器地址，如 "localhost:9092"
	LoginTopic  string        `toml:"loginTopic"`  // 登录主题（保留字段）
	LogoutTopic string        `toml:"logoutTopic"` // 登出主题（保留字段）
	ChatTopic   string        `toml:"chatTopic"`   // 聊天消息主题
	Partition   int           `toml:"partition"`   // 分区数
	Timeout     time.Duration `toml:"timeout"`     // 超时时间
}

// StaticSrcConfig 静态资源路径配置
type StaticSrcConfig struct {
	StaticAvatarPath string `toml:"staticAvatarPath"` // 头像文件存储路径
	StaticFilePath   string `toml:"staticFilePath"`   // 普通文件存储路径
}

// JWTConfig JWT 认证配置
type JWTConfig struct {
	Secret             string `toml:"secret"`             // JWT 签名密钥，建议 32 字符以上
	AccessTokenExpiry  int    `toml:"accessTokenExpiry"`  // Access Token 有效期（分钟）
	RefreshTokenExpiry int    `toml:"refreshTokenExpiry"` // Refresh Token 有效期（小时）
}

// SnowflakeConfig 雪花算法配置
type SnowflakeConfig struct {
	MachineID int64 `toml:"machineId"` // 雪花算法节点 ID，范围 0-1023，分布式部署时每台机器需唯一
}

// ModelScopeConfig ModelScope 魔搭社区配置
type ModelScopeConfig struct {
	APIKey  string `toml:"apiKey"`  // ModelScope API Key
	BaseURL string `toml:"baseUrl"` // API 基础地址
	Model   string `toml:"model"`   // 模型名称
}

// Config 应用程序总配置，聚合所有子配置
type Config struct {
	MainConfig       `toml:"mainConfig"`       // 主配置
	MysqlConfig      `toml:"mysqlConfig"`      // MySQL 配置
	RedisConfig      `toml:"redisConfig"`      // Redis 配置
	AuthCodeConfig   `toml:"authCodeConfig"`   // 短信验证码配置
	LogConfig        `toml:"logConfig"`        // 日志配置
	KafkaConfig      `toml:"kafkaConfig"`      // Kafka 配置
	StaticSrcConfig  `toml:"staticSrcConfig"`  // 静态资源配置
	JWTConfig        `toml:"jwtConfig"`        // JWT 配置
	SnowflakeConfig  `toml:"snowflakeConfig"`  // 雪花算法配置
	ModelScopeConfig `toml:"modelScopeConfig"` // ModelScope 配置
}

// config 全局配置单例
var (
	configInstance *Config
	once           sync.Once
)

// LoadConfig 从多个候选路径加载配置文件
// 按顺序尝试加载，找到第一个可用的配置文件即停止
func LoadConfig() (*Config, error) {
	newConfig := new(Config)
	// 获取程序执行目录
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)

	// 候选配置文件路径
	paths := []string{
		"configs/config_local.toml",
		"configs/config.toml",
		filepath.Join(execDir, "configs/config_local.toml"),
		filepath.Join(execDir, "configs/config.toml"),
		"/app/configs/config.toml",
		"/app/config.toml",
		"../../configs/config_local.toml",
		"../../configs/config.toml",
	}

	found := false
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, newConfig); err == nil {
				found = true
				break
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("could not find or decode configuration file in any of the search paths")
	}

	// 环境变量覆盖逻辑（优先级最高）
	overlayEnvVars(newConfig)

	return newConfig, nil
}

// overlayEnvVars 使用环境变量覆盖配置
func overlayEnvVars(c *Config) {
	if v := os.Getenv("MYSQL_HOST"); v != "" {
		c.MysqlConfig.Host = v
	}
	if v := os.Getenv("MYSQL_PASSWORD"); v != "" {
		c.MysqlConfig.Password = v
	}
	if v := os.Getenv("REDIS_HOST"); v != "" {
		c.RedisConfig.Host = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		c.RedisConfig.Password = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		c.JWTConfig.Secret = v
	}
	if v := os.Getenv("KAFKA_HOST_PORT"); v != "" {
		c.KafkaConfig.HostPort = v
	}
}

// GetConfig 获取全局配置实例（线程安全单例）
// 若加载失败会产生 fatal 错误，因为配置是程序运行的基础
func GetConfig() *Config {
	once.Do(func() {
		var err error
		configInstance, err = LoadConfig()
		if err != nil {
			// 如果配置加载失败，程序不应继续运行
			panic(fmt.Sprintf("Failed to load configuration: %v", err))
		}
	})
	return configInstance
}
