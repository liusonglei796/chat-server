// Package apply 聚合申请领域的核心业务实体、请求响应契约、存储仓储与传输协议适配
package apply

import (
	"time"

	"gorm.io/gorm"
)

// Apply 申请实体模型
// 对应数据库 contact_apply 表，存储好友申请和入群申请
type Apply struct {
	gorm.Model

	// Uuid 申请记录唯一标识 (A + 13位时间戳随机字符串)
	Uuid string `gorm:"column:uuid;uniqueIndex;type:char(20);comment:申请id"`

	// ApplicantId 申请人 UUID
	ApplicantId string `gorm:"column:applicant_id;index;uniqueIndex:idx_applicant_target_type;type:char(20);not null;comment:申请人ID"`

	// TargetId 目标 UUID (用户或群组)
	TargetId string `gorm:"column:target_id;index;uniqueIndex:idx_applicant_target_type;type:char(20);not null;comment:目标ID(用户/群组)"`

	// ContactType 申请类型: 0=申请好友, 1=申请入群
	ContactType int8 `gorm:"column:contact_type;uniqueIndex:idx_applicant_target_type;not null;comment:被申请类型，0.用户，1.群聊"`

	// Status 申请状态: 0=申请中, 1=通过, 2=拒绝, 3=拉黑
	Status int8 `gorm:"column:status;not null;comment:申请状态，0.申请中，1.通过，2.拒绝，3.拉黑"`

	// Message 申请附言
	Message string `gorm:"column:message;type:varchar(100);comment:申请信息"`

	// LastApplyAt 最后申请时间 (防止频繁申请骚扰)
	LastApplyAt time.Time `gorm:"column:last_apply_at;type:datetime;not null;comment:最后申请时间"`

	// Version 乐观锁版本号 (解决审批并发与迟到穿透)
	Version int32 `gorm:"column:version;default:1;not null;comment:乐观锁版本号"`
}

// TableName 指定表名
func (Apply) TableName() string {
	return "contact_apply"
}

// ==================== 请求与响应 DTO ====================

// ApplyFriendRequest 申请添加好友请求
type ApplyFriendRequest struct {
	FriendId    string `json:"friend_id" binding:"required"`
	Message     string `json:"message"`
	ClientToken string `json:"client_token"`
}

// ApplyGroupRequest 申请入群请求
type ApplyGroupRequest struct {
	GroupId     string `json:"group_id" binding:"required"`
	Message     string `json:"message"`
	ClientToken string `json:"client_token"`
}

// GetFriendApplyListRequest 获取好友申请列表请求
type GetFriendApplyListRequest struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// GetGroupApplyListRequest 获取入群申请列表请求
type GetGroupApplyListRequest struct {
	GroupId  string `form:"group_id" binding:"required"`
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// HandleFriendApplyRequest 处理好友申请请求（通过/拒绝/拉黑）
type HandleFriendApplyRequest struct {
	ApplicantId string `json:"applicant_id" binding:"required"`
}

// HandleGroupApplyRequest 处理入群申请请求（通过/拒绝/拉黑）
type HandleGroupApplyRequest struct {
	GroupId     string `json:"group_id" binding:"required"`
	ApplicantId string `json:"applicant_id" binding:"required"`
}

// FriendApplyListRespond 好友申请列表响应
type FriendApplyListRespond struct {
	ApplicantId     string `json:"applicant_id"`
	ApplicantName   string `json:"applicant_name"`
	ApplicantAvatar string `json:"applicant_avatar"`
	Message         string `json:"message"`
}

// GroupApplyListRespond 加群申请列表响应
type GroupApplyListRespond struct {
	ApplicantId     string `json:"applicant_id"`
	ApplicantName   string `json:"applicant_name"`
	ApplicantAvatar string `json:"applicant_avatar"`
	Message         string `json:"message"`
}

// PagedGroupApplyListRespond 分页入群申请列表响应
type PagedGroupApplyListRespond struct {
	Total int64                   `json:"total"`
	List  []GroupApplyListRespond `json:"list"`
}

// PagedFriendApplyListRespond 分页好友申请列表响应
type PagedFriendApplyListRespond struct {
	Total int64                    `json:"total"`
	List  []FriendApplyListRespond `json:"list"`
}
