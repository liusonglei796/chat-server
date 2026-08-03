// Package model 定义数据库实体模型
// 本文件定义申请模型，用于管理好友申请和入群申请
package model

import (
	"time"

	"gorm.io/gorm"
)

// Apply 申请模型
// 对应数据库 contact_apply 表
// 存储好友申请和入群申请的记录
type Apply struct {
	gorm.Model // 内嵌 GORM 模型，包含 ID、CreatedAt、UpdatedAt、DeletedAt

	// Uuid 申请记录唯一标识
	// 格式：A + 13位时间戳随机字符串
	Uuid string `gorm:"column:uuid;uniqueIndex;type:char(20);comment:申请id"`

	// ApplicantId 申请人 UUID
	// 发起申请的用户
	ApplicantId string `gorm:"column:applicant_id;index;type:char(20);not null;comment:申请人ID"`

	// TargetId 目标 UUID
	// 如果 ContactType=0，则为被申请添加好友的用户 UUID
	// 如果 ContactType=1，则为被申请加入的群组 UUID
	TargetId string `gorm:"column:target_id;index;type:char(20);not null;comment:目标ID(用户/群组)"`

	// ContactType 申请类型
	// 0=申请添加好友, 1=申请加入群聊
	ContactType int8 `gorm:"column:contact_type;not null;comment:被申请类型，0.用户，1.群聊"`

	// Status 申请状态
	// 0=申请中（待处理）
	// 1=已通过
	// 2=已拒绝
	// 3=已拉黑（不再接受该用户的申请）
	Status int8 `gorm:"column:status;not null;comment:申请状态，0.申请中，1.通过，2.拒绝，3.拉黑"`

	// Message 申请附言
	// 申请时附带的验证消息
	Message string `gorm:"column:message;type:varchar(100);comment:申请信息"`

	// LastApplyAt 最后申请时间
	// 用于判断是否可以再次申请（防止频繁骚扰）
	LastApplyAt time.Time `gorm:"column:last_apply_at;type:datetime;not null;comment:最后申请时间"`
}

// TableName 指定表名
func (Apply) TableName() string {
	return "contact_apply"
}
