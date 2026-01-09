// Package mysql 提供数据访问层的初始化和全局数据库实例管理
// 负责建立 MySQL 连接、自动迁移表结构、初始化 Repository 层
package mysql

import (
	"fmt"

	"kama_chat_server/internal/config" // 配置管理
	"kama_chat_server/internal/model"  // 数据模型

	"go.uber.org/zap"                  // 日志库
	mysqldriver "gorm.io/driver/mysql" // GORM MySQL 驱动
	"gorm.io/gorm"                     // GORM ORM 框架
)

// Init 初始化数据库连接并返回 Repository 层实例
// 执行步骤：
//  1. 从配置读取 MySQL 连接信息
//  2. 构建 DSN（Data Source Name）连接字符串
//  3. 使用 GORM 建立数据库连接
//  4. 执行 AutoMigrate 自动迁移表结构
//  5. 创建并返回 Repository 实例
//
// 返回: Repository 实例集合
func Init() *Repositories {
	// 获取配置
	conf := config.GetConfig()

	// 构建 MySQL DSN 连接字符串
	// 格式：user:password@tcp(host:port)/database?params
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		conf.MysqlConfig.User,         // 用户名
		conf.MysqlConfig.Password,     // 密码
		conf.MysqlConfig.Host,         // 主机地址
		conf.MysqlConfig.Port,         // 端口
		conf.MysqlConfig.DatabaseName, // 数据库名
	)

	// 使用 GORM 打开数据库连接
	db, err := gorm.Open(mysqldriver.Open(dsn), &gorm.Config{})
	if err != nil {
		// 连接失败，记录致命错误并退出程序
		zap.L().Fatal(err.Error())
	}

	// AutoMigrate 自动迁移表结构
	// 如果表不存在则创建，如果字段变更则更新结构
	// 注意：不会删除已有字段或数据
	err = db.AutoMigrate(
		&model.UserInfo{},    // 用户信息表
		&model.GroupInfo{},   // 群组信息表
		&model.Contact{},     // 用户联系人表
		&model.Session{},     // 会话表
		&model.Apply{},       // 联系人申请表
		&model.Message{},     // 消息表
		&model.GroupMember{}, // 群组成员表
	)
	if err != nil {
		// 迁移失败，记录致命错误并退出程序
		zap.L().Fatal(err.Error())
	}

	// 创建并返回 Repository 实例集合
	return NewRepositories(db)
}
