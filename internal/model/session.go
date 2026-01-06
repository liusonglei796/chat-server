// Package model 定义数据库实体模型
// 本文件定义会话模型，用于管理用户之间的聊天会话
package model

import (
	"database/sql"

	"gorm.io/gorm"
)

// Session 会话模型
// 对应数据库 session 表
// 会话代表两个实体（用户或群组）之间的聊天关系
type Session struct {
	gorm.Model // 内嵌 GORM 模型，包含 ID、CreatedAt、UpdatedAt、DeletedAt

	// Uuid 会话唯一标识
	// 格式：S + 13位时间戳随机字符串
	Uuid string `gorm:"column:uuid;uniqueIndex;type:char(20);comment:会话uuid"`

	// SendId 创建会话的用户 UUID
	// 即主动发起聊天的一方
	SendId string `gorm:"column:send_id;index;type:char(20);not null;comment:创建会话人id"`

	// ReceiveId 接收会话的实体 UUID
	// 可以是用户 UUID（U开头）或群组 UUID（G开头）
	ReceiveId string `gorm:"column:receive_id;index;type:char(20);not null;comment:接受会话人id"`

	// ReceiveName 接收方名称
	// 冗余存储，用于会话列表显示
	// 如果是用户则为昵称，如果是群组则为群名
	ReceiveName string `gorm:"column:receive_name;type:varchar(20);not null;comment:名称"`

	// Avatar 接收方头像
	// 冗余存储，用于会话列表显示
	Avatar string `gorm:"column:avatar;type:char(255);default:default_avatar.png;not null;comment:头像"`

	// LastMessage 最新消息内容
	// 用于在会话列表中显示最后一条消息摘要
	LastMessage string `gorm:"column:last_message;type:TEXT;comment:最新的消息"`

	// LastMessageAt 最后消息时间
	// 用于会话列表排序（最近聊天的排在前面）
	LastMessageAt sql.NullTime `gorm:"column:last_message_at;type:datetime;comment:最近接收时间"`
}

// TableName 指定表名
func (Session) TableName() string {
	return "session"
}
