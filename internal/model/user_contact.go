package model

import (
	"gorm.io/gorm"
)

type UserContact struct {
	gorm.Model
	UserId      string `gorm:"column:user_id;index;type:char(20);not null;comment:用户唯一id"`
	ContactId   string `gorm:"column:contact_id;index;type:char(20);not null;comment:联系人ID"`
	ContactType int8   `gorm:"column:contact_type;not null;comment:联系类型，0.用户，1.群聊"`
	Status      int8   `gorm:"column:status;not null;comment:联系状态，0.正常，1.拉黑，2.被拉黑，3.删除好友，4.被删除好友，5.被禁言，6.退出群聊，7.被踢出群聊"`
}

func (UserContact) TableName() string {
	return "user_contact"
}
