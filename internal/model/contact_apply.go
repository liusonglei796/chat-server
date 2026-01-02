package model

import (
	"time"

	"gorm.io/gorm"
)

type ContactApply struct {
	gorm.Model
	Uuid        string    `gorm:"column:uuid;uniqueIndex;type:char(20);comment:申请id"`
	ApplicantId string    `gorm:"column:applicant_id;index;type:char(20);not null;comment:申请人ID"`
	TargetId    string    `gorm:"column:target_id;index;type:char(20);not null;comment:目标ID(用户/群组)"`
	ContactType int8      `gorm:"column:contact_type;not null;comment:被申请类型，0.用户，1.群聊"`
	Status      int8      `gorm:"column:status;not null;comment:申请状态，0.申请中，1.通过，2.拒绝，3.拉黑"`
	Message     string    `gorm:"column:message;type:varchar(100);comment:申请信息"`
	LastApplyAt time.Time `gorm:"column:last_apply_at;type:datetime;not null;comment:最后申请时间"`
}

func (ContactApply) TableName() string {
	return "contact_apply"
}
