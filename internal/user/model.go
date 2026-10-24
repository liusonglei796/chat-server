package user

import (
	"database/sql"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserInfo 用户信息模型
// 对应数据库 user_info 表
// 显式声明主键与时间戳，严禁使用 gorm.Model（消除隐式软删除与全局 WHERE deleted_at IS NULL 注入）
type UserInfo struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`

	// Uuid 用户唯一标识
	Uuid string `gorm:"column:uuid;uniqueIndex;type:char(20);comment:用户唯一id"`

	// Nickname 用户昵称
	Nickname string `gorm:"column:nickname;uniqueIndex;type:varchar(20);not null;comment:昵称"`

	// Telephone 手机号码
	Telephone string `gorm:"column:telephone;index;not null;type:char(11);comment:电话"`

	// Email 邮箱地址（可选）
	Email string `gorm:"column:email;type:char(30);comment:邮箱"`

	// Avatar 用户头像 URL
	Avatar string `gorm:"column:avatar;type:char(255);default:https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png;not null;comment:头像"`

	// Gender 性别 (0=男, 1=女)
	Gender int8 `gorm:"column:gender;comment:性别，0.男，1.女"`

	// Signature 个性签名
	Signature string `gorm:"column:signature;type:varchar(100);comment:个性签名"`

	// Password 密码（已哈希）
	Password string `gorm:"column:password;type:varchar(100);not null;comment:密码"`

	// Birthday 生日 (YYYYMMDD)
	Birthday string `gorm:"column:birthday;type:char(8);comment:生日"`

	// LastOnlineAt 上次登录时间
	LastOnlineAt sql.NullTime `gorm:"column:last_online_at;type:datetime;comment:上次登录时间"`

	// LastOfflineAt 最近离线时间
	LastOfflineAt sql.NullTime `gorm:"column:last_offline_at;type:datetime;comment:最近离线时间"`

	// IsAdmin 管理员标志 (0=普通用户, 1=管理员)
	IsAdmin int8 `gorm:"column:is_admin;not null;comment:是否是管理员，0.不是，1.是"`

	// Status 账号状态 (0=正常, 1=禁用)
	Status int8 `gorm:"column:status;index;not null;comment:状态，0.正常，1.禁用"`

	// Version 乐观锁版本号
	Version int32 `gorm:"column:version;default:1;not null;comment:乐观锁版本号"`

	// RawPassword 明文密码（不存入数据库，BeforeSave 自动加密）
	RawPassword string `gorm:"-" json:"-"`
}

// TableName 指定表名
func (UserInfo) TableName() string {
	return "user_info"
}

// BeforeSave GORM Hook
func (u *UserInfo) BeforeSave(tx *gorm.DB) (err error) {
	if u.RawPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(u.RawPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.Password = string(hash)
		u.RawPassword = ""
	}
	return nil
}

// CheckPassword 校验密码是否正确
func (u *UserInfo) CheckPassword(plaintext string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(plaintext))
	return err == nil
}

// ==================== 请求与响应 DTO ====================

// LoginRequest 用户登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RefreshTokenRequest 刷新令牌请求
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// RegisterRequest 用户注册请求
type RegisterRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	ClientToken string `json:"client_token"`
}

// GetUserInfoRequest 获取用户信息请求
type GetUserInfoRequest struct {
	Uuid string `json:"uuid" form:"uuid" binding:"required"`
}

// UpdateUserInfoRequest 更新用户信息请求
type UpdateUserInfoRequest struct {
	Uuid      string  `json:"uuid"`
	Email     *string `json:"email"`
	Nickname  *string `json:"nickname"`
	Birthday  *string `json:"birthday"`
	Signature *string `json:"signature"`
	Avatar    *string `json:"avatar"`
	Version   *int32  `json:"version"`
}

// LoginRespond 登录成功响应
type LoginRespond struct {
	Uuid         string `json:"uuid"`
	Telephone    string `json:"telephone"`
	Nickname     string `json:"nickname"`
	Email        string `json:"email"`
	Avatar       string `json:"avatar"`
	Gender       int8   `json:"gender"`
	Birthday     string `json:"birthday"`
	Signature    string `json:"signature"`
	IsAdmin      int8   `json:"is_admin"`
	Status       int8   `json:"status"`
	CreatedAt    string `json:"created_at"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// RegisterRespond 注册成功响应
type RegisterRespond struct {
	Uuid      string `json:"uuid"`
	Telephone string `json:"telephone"`
	Nickname  string `json:"nickname"`
	Email     string `json:"email"`
	Avatar    string `json:"avatar"`
	Gender    int8   `json:"gender"`
	Birthday  string `json:"birthday"`
	Signature string `json:"signature"`
	CreatedAt string `json:"created_at"`
	IsAdmin   int8   `json:"is_admin"`
	Status    int8   `json:"status"`
	Version   int32  `json:"version"`
}

// GetUserInfoRespond 获取用户信息响应
type GetUserInfoRespond struct {
	Uuid      string `json:"uuid"`
	Telephone string `json:"telephone"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	Birthday  string `json:"birthday"`
	Email     string `json:"email"`
	Gender    int8   `json:"gender"`
	Signature string `json:"signature"`
	CreatedAt string `json:"created_at"`
	IsAdmin   int8   `json:"is_admin"`
	Status    int8   `json:"status"`
	Version   int32  `json:"version"`
}

// PublicUserInfoRespond 公开用户信息响应
type PublicUserInfoRespond struct {
	Uuid      string `json:"uuid"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	Gender    int8   `json:"gender"`
	Birthday  string `json:"birthday"`
	Signature string `json:"signature"`
}
