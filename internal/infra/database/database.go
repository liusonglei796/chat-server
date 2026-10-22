// Package database 提供通用的 MySQL GORM 数据库连接管理与迁移工具
package database

import (
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	mysqldriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"kama_chat_server/internal/infra/config"
)

// Open 根据配置与指定的数据库名称初始化 GORM 实例
func Open(conf *config.MysqlConfig, dbName string) (*gorm.DB, error) {
	if dbName == "" {
		dbName = conf.DatabaseName
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		conf.User, conf.Password,
		conf.Host, conf.Port, dbName)

	db, err := gorm.Open(mysqldriver.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database %s: %w", dbName, err)
	}

	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
	}

	return db, nil
}

// AutoMigrate 为指定数据库实例迁移模型表结构，容忍 1050 (table already exists) 偶发竞态
func AutoMigrate(db *gorm.DB, models ...interface{}) error {
	if len(models) == 0 {
		return nil
	}
	err := db.AutoMigrate(models...)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1050 {
			err = db.AutoMigrate(models...)
		}
		if err != nil {
			zap.L().Error("AutoMigrate failed", zap.Error(err))
			return err
		}
	}
	return nil
}
