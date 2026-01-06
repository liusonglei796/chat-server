// Package config 提供应用程序的配置加载和管理功能
// 使用 TOML 格式的配置文件，支持多路径查找
package config

import (
	"fmt"
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
	MessageMode string        `toml:"messageMode"` // 消息模式："channel" 或 "kafka"
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

// Config 应用程序总配置，聚合所有子配置
type Config struct {
	MainConfig      `toml:"mainConfig"`      // 主配置
	MysqlConfig     `toml:"mysqlConfig"`     // MySQL 配置
	RedisConfig     `toml:"redisConfig"`     // Redis 配置
	AuthCodeConfig  `toml:"authCodeConfig"`  // 短信验证码配置
	LogConfig       `toml:"logConfig"`       // 日志配置
	KafkaConfig     `toml:"kafkaConfig"`     // Kafka 配置
	StaticSrcConfig `toml:"staticSrcConfig"` // 静态资源配置
	JWTConfig       `toml:"jwtConfig"`       // JWT 配置
	SnowflakeConfig `toml:"snowflakeConfig"` // 雪花算法配置
}

// config 全局配置单例，延迟加载
var config *Config

// LoadConfig 从多个候选路径加载配置文件
// 按顺序尝试加载，找到第一个可用的配置文件即停止
// 返回值：加载成功返回 nil，否则返回错误
func LoadConfig() error {
	// 候选配置文件路径（优先加载本地配置）
	paths := []string{
		"configs/config_local.toml",      // 本地开发配置（优先）
		"configs/config.toml",            // 默认配置
		"../../configs/config_local.toml", // 从子目录运行时的路径
		"../../configs/config.toml",       // 从子目录运行时的路径
	}

	// 依次尝试加载配置文件
	for _, path := range paths {
		if _, err := toml.DecodeFile(path, config); err == nil {
			return nil // 加载成功
		}
	}

	return fmt.Errorf("could not find configuration file in any of the search paths")
}

// GetConfig 获取全局配置实例（单例模式）
// 首次调用时会自动加载配置文件
func GetConfig() *Config {
	if config == nil {
		config = new(Config)
		_ = LoadConfig() // 忽略加载错误，使用默认值
	}
	return config
}
