package model

import "gorm.io/gorm"

// GroupMember 群成员关联表
type GroupMember struct {
	gorm.Model
	GroupUuid string `gorm:"type:char(20);index;not null;comment:群组ID"`
	UserUuid  string `gorm:"type:char(20);index;not null;comment:用户ID"`
	Role      int8   `gorm:"default:1;comment:1普通成员 2管理员 3群主"`
}

func (GroupMember) TableName() string {
	return "group_member"
}
