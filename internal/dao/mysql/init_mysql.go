// Package mysql 提供数据访问层的初始化和全局数据库实例管理
// 负责建立 MySQL 连接、自动迁移表结构、初始化 Repository 层
package mysql

import (
	"errors"
	"fmt"

	"kama_chat_server/internal/config" // 配置管理
	"kama_chat_server/internal/model"  // 数据模型

	"github.com/go-sql-driver/mysql"   // MySQL 驱动错误码
	"go.uber.org/zap"                  // 日志库
	mysqldriver "gorm.io/driver/mysql" // GORM MySQL 驱动
	"gorm.io/gorm"                     // GORM ORM 框架
)

// ServiceModels 每个服务只迁移自己库中的表
var ServiceModels = map[string][]interface{}{
	"user": {
		&model.UserInfo{}, &model.Outbox{},
	},
	"relation": {
		&model.GroupInfo{}, &model.GroupMember{},
		&model.Friendship{}, &model.Apply{}, &model.Outbox{},
	},
	"message": {
		&model.Session{}, &model.Message{}, &model.Outbox{},
	},
}

// InitFor 初始化指定服务对应的库表迁移
func InitFor(service string) *Repositories {
	conf := config.GetConfig()
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		conf.MysqlConfig.User, conf.MysqlConfig.Password,
		conf.MysqlConfig.Host, conf.MysqlConfig.Port, conf.MysqlConfig.DatabaseName)
	db, err := gorm.Open(mysqldriver.Open(dsn), &gorm.Config{})
	if err != nil {
		zap.L().Fatal(err.Error())
	}
	models, ok := ServiceModels[service]
	if !ok {
		zap.L().Fatal("unknown service", zap.String("service", service))
	}
	err = db.AutoMigrate(models...)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1050 {
			err = db.AutoMigrate(models...)
		}
		if err != nil {
			zap.L().Fatal(err.Error())
		}
	}
	return NewRepositories(db)
}
