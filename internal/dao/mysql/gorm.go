package dao

import (
	"fmt"
	"kama_chat_server/internal/config"
	"kama_chat_server/internal/dao/mysql/repository"
	"kama_chat_server/internal/model"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var GormDB *gorm.DB

// Repos 全局 Repository 实例，供 Service 层使用
var Repos *repository.Repositories

// Init 初始化数据库连接
func Init() {
	conf := config.GetConfig()
	// password := conf.MysqlConfig.Password
	// host := conf.MysqlConfig.Host
	// port := conf.MysqlConfig.Port
	// appName := conf.AppName
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		conf.MysqlConfig.User,
		conf.MysqlConfig.Password,
		conf.MysqlConfig.Host,
		conf.MysqlConfig.Port,
		conf.MysqlConfig.DatabaseName,
	)
	// dsn := fmt.Sprintf("%s@unix(/var/run/mysqld/mysqld.sock)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, appName)
	var err error
	GormDB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		zap.L().Fatal(err.Error())
	}
	err = GormDB.AutoMigrate(&model.UserInfo{}, &model.GroupInfo{}, &model.UserContact{}, &model.Session{}, &model.ContactApply{}, &model.Message{}, &model.GroupMember{}) // 自动迁移，如果没有建表，会自动创建对应的表
	if err != nil {
		zap.L().Fatal(err.Error())
	}

	// 初始化全局 Repositories
	Repos = repository.NewRepositories(GormDB)
}
