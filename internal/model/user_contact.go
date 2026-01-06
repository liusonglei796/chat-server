// Package model 定义数据库实体模型
// 本文件定义用户联系人模型，用于管理好友和群组关系
package model

import (
	"gorm.io/gorm"
)

// UserContact 用户联系人模型
// 对应数据库 user_contact 表
// 存储用户的好友关系和加入的群组
type UserContact struct {
	gorm.Model // 内嵌 GORM 模型，包含 ID、CreatedAt、UpdatedAt、DeletedAt

	// UserId 用户 UUID
	// 表示这条记录属于哪个用户
	UserId string `gorm:"column:user_id;index;type:char(20);not null;comment:用户唯一id"`

	// ContactId 联系人/群组 UUID
	// 如果 ContactType=0，则为另一个用户的 UUID
	// 如果 ContactType=1，则为群组的 UUID
	ContactId string `gorm:"column:contact_id;index;type:char(20);not null;comment:联系人ID"`

	// ContactType 联系类型
	// 0=用户（好友关系）, 1=群聊（已加入的群）
	ContactType int8 `gorm:"column:contact_type;not null;comment:联系类型，0.用户，1.群聊"`

	// Status 联系状态
	// 0=正常：正常好友/群成员关系
	// 1=拉黑：我拉黑了对方
	// 2=被拉黑：被对方拉黑
	// 3=删除好友：我删除了好友
	// 4=被删除好友：被对方删除
	// 5=被禁言：在群中被禁言
	// 6=退出群聊：主动退群
	// 7=被踢出群聊：被管理员踢出
	Status int8 `gorm:"column:status;not null;comment:联系状态，0.正常，1.拉黑，2.被拉黑，3.删除好友，4.被删除好友，5.被禁言，6.退出群聊，7.被踢出群聊"`
}

// TableName 指定表名
func (UserContact) TableName() string {
	return "user_contact"
}
