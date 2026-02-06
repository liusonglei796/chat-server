// Package model 定义数据库实体模型
// 本文件定义好友关系模型，用于管理用户之间的好友关系

package model

import (
	"gorm.io/gorm"
)

// Friendship 好友关系模型
// 对应数据库 user_friendship 表
// 仅存储用户之间的好友关系（群组关系由 GroupMember 表管理）
// user_friendship 是已建立的好友关系表；apply 是申请阶段的记录表（好友/入群申请）。
type Friendship struct {
	gorm.Model // 内嵌 GORM 模型，包含 ID、CreatedAt、UpdatedAt、DeletedAt

	// UserId 用户 UUID
	// 表示这条记录属于哪个用户
	UserId string `gorm:"column:user_id;index;type:char(20);not null;uniqueIndex:idx_user_friend;comment:用户唯一id"`

	// FriendId 好友用户 UUID
	FriendId string `gorm:"column:friend_id;index;type:char(20);not null;uniqueIndex:idx_user_friend;comment:好友ID"`

	// Status 好友状态
	// 0=正常：正常好友关系
	// 1=拉黑：我拉黑了对方
	// 2=被拉黑：被对方拉黑
	Status int8 `gorm:"column:status;not null;comment:好友状态，0.正常，1.拉黑，2.被拉黑"`
}

// TableName 指定表名
func (Friendship) TableName() string {
	return "user_friendship"
}
