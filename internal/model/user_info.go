package model

import (
	"database/sql"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserInfo struct {
	gorm.Model
	Uuid          string       `gorm:"column:uuid;uniqueIndex;type:char(20);comment:用户唯一id"`
	Nickname      string       `gorm:"column:nickname;type:varchar(20);not null;comment:昵称"`
	Telephone     string       `gorm:"column:telephone;index;not null;type:char(11);comment:电话"`
	Email         string       `gorm:"column:email;type:char(30);comment:邮箱"`
	Avatar        string       `gorm:"column:avatar;type:char(255);default:https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png;not null;comment:头像"`
	Gender        int8         `gorm:"column:gender;comment:性别，0.男，1.女"`
	Signature     string       `gorm:"column:signature;type:varchar(100);comment:个性签名"`
	Password      string       `gorm:"column:password;type:varchar(100);not null;comment:密码"`
	Birthday      string       `gorm:"column:birthday;type:char(8);comment:生日"`
	LastOnlineAt  sql.NullTime `gorm:"column:last_online_at;type:datetime;comment:上次登录时间"`
	LastOfflineAt sql.NullTime `gorm:"column:last_offline_at;type:datetime;comment:最近离线时间"`
	IsAdmin       int8         `gorm:"column:is_admin;not null;comment:是否是管理员，0.不是，1.是"`
	Status        int8         `gorm:"column:status;index;not null;comment:状态，0.正常，1.禁用"`
	RawPassword   string       `gorm:"-" json:"-"` // 不存库，仅用于接收明文密码
}

func (UserInfo) TableName() string {
	return "user_info"
}

// BeforeSave 在创建和更新前自动加密密码
func (u *UserInfo) BeforeSave(tx *gorm.DB) (err error) {
	if u.RawPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(u.RawPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.Password = string(hash)
		u.RawPassword = "" // 清空明文
	}
	return nil
}

// CheckPassword 校验密码是否正确
func (u *UserInfo) CheckPassword(plaintext string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(plaintext))
	return err == nil
}
