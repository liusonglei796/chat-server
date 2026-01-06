// Package model 定义数据库实体模型
// 本文件定义用户信息模型，包含用户基本资料和认证信息
package model

import (
	"database/sql"

	"golang.org/x/crypto/bcrypt" // 密码哈希库
	"gorm.io/gorm"
)

// UserInfo 用户信息模型
// 对应数据库 user_info 表
type UserInfo struct {
	gorm.Model // 内嵌 GORM 模型，包含 ID、CreatedAt、UpdatedAt、DeletedAt

	// Uuid 用户唯一标识
	// 格式：U + 13位时间戳随机字符串，如 "U2024010412345678"
	Uuid string `gorm:"column:uuid;uniqueIndex;type:char(20);comment:用户唯一id"`

	// Nickname 用户昵称
	Nickname string `gorm:"column:nickname;type:varchar(20);not null;comment:昵称"`

	// Telephone 手机号码
	// 用于登录验证，建立索引加速查询
	Telephone string `gorm:"column:telephone;index;not null;type:char(11);comment:电话"`

	// Email 邮箱地址（可选）
	Email string `gorm:"column:email;type:char(30);comment:邮箱"`

	// Avatar 用户头像 URL
	// 默认使用饿了么 CDN 的默认头像
	Avatar string `gorm:"column:avatar;type:char(255);default:https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png;not null;comment:头像"`

	// Gender 性别
	// 0=男, 1=女
	Gender int8 `gorm:"column:gender;comment:性别，0.男，1.女"`

	// Signature 个性签名
	Signature string `gorm:"column:signature;type:varchar(100);comment:个性签名"`

	// Password 密码（已哈希）
	// 存储 bcrypt 哈希后的密码，不存储明文
	Password string `gorm:"column:password;type:varchar(100);not null;comment:密码"`

	// Birthday 生日
	// 格式：YYYYMMDD
	Birthday string `gorm:"column:birthday;type:char(8);comment:生日"`

	// LastOnlineAt 上次登录时间
	LastOnlineAt sql.NullTime `gorm:"column:last_online_at;type:datetime;comment:上次登录时间"`

	// LastOfflineAt 最近离线时间
	LastOfflineAt sql.NullTime `gorm:"column:last_offline_at;type:datetime;comment:最近离线时间"`

	// IsAdmin 管理员标志
	// 0=普通用户, 1=管理员
	IsAdmin int8 `gorm:"column:is_admin;not null;comment:是否是管理员，0.不是，1.是"`

	// Status 账号状态
	// 0=正常, 1=禁用
	Status int8 `gorm:"column:status;index;not null;comment:状态，0.正常，1.禁用"`

	// RawPassword 明文密码（不存入数据库）
	// 用于接收前端传来的明文密码，在 BeforeSave 中加密
	// gorm:"-" 表示忽略此字段，不进行数据库操作
	RawPassword string `gorm:"-" json:"-"`
}

// TableName 指定表名
// GORM 默认会将结构体名转为蛇形命名，这里显式指定
func (UserInfo) TableName() string {
	return "user_info"
}

// BeforeSave GORM Hook：在创建和更新前自动调用
// 作用：将 RawPassword 明文密码加密后存入 Password 字段
// 这样调用方只需设置 RawPassword，无需手动加密
func (u *UserInfo) BeforeSave(tx *gorm.DB) (err error) {
	// 如果提供了明文密码，则进行加密
	if u.RawPassword != "" {
		// 使用 bcrypt 算法加密，DefaultCost=10
		hash, err := bcrypt.GenerateFromPassword([]byte(u.RawPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.Password = string(hash) // 存储加密后的密码
		u.RawPassword = ""        // 清空明文，防止泄露
	}
	return nil
}

// CheckPassword 校验密码是否正确
// 用于登录时验证用户输入的密码
// plaintext: 用户输入的明文密码
// 返回: 密码是否正确
func (u *UserInfo) CheckPassword(plaintext string) bool {
	// CompareHashAndPassword 比较哈希密码和明文
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(plaintext))
	return err == nil // 无错误表示密码正确
}
