// Package model 定义数据库实体模型
// 本文件定义群成员关联模型
package model

import "gorm.io/gorm"

// GroupMember 群成员关联模型
// 对应数据库 group_member 表
// 用于存储群组与用户的多对多关系
type GroupMember struct {
	gorm.Model // 内嵌 GORM 模型，包含 ID、CreatedAt、UpdatedAt、DeletedAt

	// GroupUuid 群组 UUID
	// 关联到 GroupInfo 表
	GroupUuid string `gorm:"type:char(20);index;not null;comment:群组ID"`

	// UserUuid 用户 UUID
	// 关联到 UserInfo 表
	UserUuid string `gorm:"type:char(20);index;not null;comment:用户ID"`

	// Role 成员角色
	// 1=普通成员, 2=管理员, 3=群主
	// 群主在创建群时自动设置，管理员由群主指定
	Role int8 `gorm:"default:1;comment:1普通成员 2管理员 3群主"`
}

// TableName 指定表名
func (GroupMember) TableName() string {
	return "group_member"
}
