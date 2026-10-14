// Package model 定义数据库实体模型
// 本文件定义事务发件箱模型，用于跨服务领域事件的可靠投递
package model

import "time"

// Outbox 事务发件箱
// 与业务写操作在同一本地事务中落库，由发布器轮询发送到 Kafka 后标记 published
// 刻意不内嵌 gorm.Model：避免软删除字段干扰重发语义
type Outbox struct {
	Uuid        string    `gorm:"column:uuid;primaryKey;type:char(20);comment:事件ID"`
	AggregateId string    `gorm:"column:aggregate_id;type:varchar(64);not null;default:'';index;comment:业务聚合根ID"`
	EventType   string    `gorm:"column:event_type;type:varchar(50);not null;index;comment:事件类型"`
	Payload     string    `gorm:"column:payload;type:text;not null;comment:事件JSON负载"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;comment:创建时间"`
}

// TableName 指定表名
func (Outbox) TableName() string {
	return "outbox"
}
