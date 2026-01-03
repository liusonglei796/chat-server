package config

import (
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
)

type MainConfig struct {
	AppName string `toml:"appName"`
	Host    string `toml:"host"`
	Port    int    `toml:"port"`
}

type MysqlConfig struct {
	Host         string `toml:"host"`
	Port         int    `toml:"port"`
	User         string `toml:"user"`
	Password     string `toml:"password"`
	DatabaseName string `toml:"databaseName"`
}

type RedisConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Password string `toml:"password"`
	Db       int    `toml:"db"`
}

type AuthCodeConfig struct {
	AccessKeyID     string `toml:"accessKeyID"`
	AccessKeySecret string `toml:"accessKeySecret"`
	SignName        string `toml:"signName"`
	TemplateCode    string `toml:"templateCode"`
}

type LogConfig struct {
	LogPath    string `toml:"logPath"`
	FileName   string `toml:"fileName"`   // 日志文件名
	MaxSize    int    `toml:"maxSize"`    // 单个日志文件最大大小（MB）
	MaxBackups int    `toml:"maxBackups"` // 保留旧日志文件的最大个数
	MaxAge     int    `toml:"maxAge"`     // 保留旧日志文件的最大天数
	Level      string `toml:"level"`      // 日志级别：debug, info, warn, error
}

type KafkaConfig struct {
	MessageMode string        `toml:"messageMode"`
	HostPort    string        `toml:"hostPort"`
	LoginTopic  string        `toml:"loginTopic"`
	LogoutTopic string        `toml:"logoutTopic"`
	ChatTopic   string        `toml:"chatTopic"`
	Partition   int           `toml:"partition"`
	Timeout     time.Duration `toml:"timeout"`
}

type StaticSrcConfig struct {
	StaticAvatarPath string `toml:"staticAvatarPath"`
	StaticFilePath   string `toml:"staticFilePath"`
}

type JWTConfig struct {
	Secret             string `toml:"secret"`
	AccessTokenExpiry  int    `toml:"accessTokenExpiry"`  // 分钟
	RefreshTokenExpiry int    `toml:"refreshTokenExpiry"` // 小时
}

type SnowflakeConfig struct {
	MachineID int64 `toml:"machineId"` // 雪花算法节点 ID (0-1023)
}

type Config struct {
	MainConfig      `toml:"mainConfig"`
	MysqlConfig     `toml:"mysqlConfig"`
	RedisConfig     `toml:"redisConfig"`
	AuthCodeConfig  `toml:"authCodeConfig"`
	LogConfig       `toml:"logConfig"`
	KafkaConfig     `toml:"kafkaConfig"`
	StaticSrcConfig `toml:"staticSrcConfig"`
	JWTConfig       `toml:"jwtConfig"`
	SnowflakeConfig `toml:"snowflakeConfig"`
}

var config *Config

func LoadConfig() error {
	paths := []string{
		"configs/config_local.toml",
		"configs/config.toml",
		"../../configs/config_local.toml",
		"../../configs/config.toml",
	}

	for _, path := range paths {
		if _, err := toml.DecodeFile(path, config); err == nil {
			return nil
		}
	}

	return fmt.Errorf("could not find configuration file in any of the search paths")
}

func GetConfig() *Config {
	if config == nil {
		config = new(Config)
		_ = LoadConfig()
	}
	return config
}
