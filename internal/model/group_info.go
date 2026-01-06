// Package model 定义数据库实体模型
// 本文件定义群组信息模型
package model

import (
	"gorm.io/gorm"
)

// GroupInfo 群组信息模型
// 对应数据库 group_info 表
type GroupInfo struct {
	gorm.Model // 内嵌 GORM 模型，包含 ID、CreatedAt、UpdatedAt、DeletedAt

	// Uuid 群组唯一标识
	// 格式：G + 13位时间戳随机字符串，如 "G2024010412345678"
	Uuid string `gorm:"column:uuid;uniqueIndex;type:char(20);not null;comment:群组唯一id"`

	// Name 群组名称
	Name string `gorm:"column:name;type:varchar(20);not null;comment:群名称"`

	// Notice 群公告
	// 群主或管理员可编辑
	Notice string `gorm:"column:notice;type:varchar(500);comment:群公告"`

	// MemberCnt 群成员数量
	// 默认为 1（创建者），通过 IncrementMemberCount/DecrementMemberCount 更新
	MemberCnt int `gorm:"column:member_cnt;default:1;comment:群人数"`

	// OwnerId 群主用户 UUID
	OwnerId string `gorm:"column:owner_id;type:char(20);not null;comment:群主uuid"`

	// AddMode 加群方式
	// 0=直接加入（无需审批）, 1=需要审核
	AddMode int8 `gorm:"column:add_mode;default:0;comment:加群方式，0.直接，1.审核"`

	// Avatar 群头像 URL
	// 默认使用饿了么 CDN 的默认头像
	Avatar string `gorm:"column:avatar;type:char(255);default:https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png;not null;comment:头像"`

	// Status 群状态
	// 0=正常, 1=禁用（管理员操作）, 2=解散
	Status int8 `gorm:"column:status;default:0;comment:状态，0.正常，1.禁用，2.解散"`
}

// TableName 指定表名
func (GroupInfo) TableName() string {
	return "group_info"
}
