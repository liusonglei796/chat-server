package model

import (
	"database/sql"

	"gorm.io/gorm"
)

type Session struct {
	gorm.Model
	Uuid          string       `gorm:"column:uuid;uniqueIndex;type:char(20);comment:会话uuid"`
	SendId        string       `gorm:"column:send_id;index;type:char(20);not null;comment:创建会话人id"`
	ReceiveId     string       `gorm:"column:receive_id;index;type:char(20);not null;comment:接受会话人id"`
	ReceiveName   string       `gorm:"column:receive_name;type:varchar(20);not null;comment:名称"`
	Avatar        string       `gorm:"column:avatar;type:char(255);default:default_avatar.png;not null;comment:头像"`
	LastMessage   string       `gorm:"column:last_message;type:TEXT;comment:最新的消息"`
	LastMessageAt sql.NullTime `gorm:"column:last_message_at;type:datetime;comment:最近接收时间"`
}

func (Session) TableName() string {
	return "session"
}
