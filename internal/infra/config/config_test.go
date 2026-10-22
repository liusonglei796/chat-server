package config

import (
	"os"
	"testing"
)

func TestLoadConfig_EnvOverlay(t *testing.T) {
	// 1. 设置环境变量
	os.Setenv("MYSQL_PASSWORD", "env_secret_password")
	defer os.Unsetenv("MYSQL_PASSWORD")

	// 2. 加载配置 (这里会尝试加载真实的配置文件，然后覆盖)
	conf, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// 3. 校验环境变量是否生效
	if conf.MysqlConfig.Password != "env_secret_password" {
		t.Errorf("Expected MYSQL_PASSWORD to be 'env_secret_password', got '%s'", conf.MysqlConfig.Password)
	}
}

func TestLoadConfig_EnvDBOverlay(t *testing.T) {
	os.Setenv("MYSQL_USER", "svc_message")
	os.Setenv("DB_NAME", "chat_message")
	defer os.Unsetenv("MYSQL_USER")
	defer os.Unsetenv("DB_NAME")

	conf, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if conf.MysqlConfig.User != "svc_message" {
		t.Errorf("expected MYSQL_USER overlay, got %q", conf.MysqlConfig.User)
	}
	if conf.MysqlConfig.DatabaseName != "chat_message" {
		t.Errorf("expected DB_NAME overlay, got %q", conf.MysqlConfig.DatabaseName)
	}
}

func TestGetConfig_Singleton(t *testing.T) {
	// 获取两次实例，校验是否为同一个指针
	c1 := GetConfig()
	c2 := GetConfig()

	if c1 != c2 {
		t.Error("GetConfig should return the same instance (singleton)")
	}
}
