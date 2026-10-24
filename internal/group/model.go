package group

import (
	"database/sql"
	"time"
)

// GroupInfo 群组信息模型
// 对应数据库 group_info 表
// 显式声明主键与时间戳，严禁使用 gorm.Model（消除隐式软删除与全局 WHERE deleted_at IS NULL 注入）
type GroupInfo struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`

	// Uuid 群组唯一标识
	Uuid string `gorm:"column:uuid;uniqueIndex;type:char(20);not null;comment:群组唯一id"`

	// Name 群组名称
	Name string `gorm:"column:name;uniqueIndex:idx_owner_name;type:varchar(20);not null;comment:群名称"`

	// Notice 群公告
	Notice string `gorm:"column:notice;type:varchar(500);comment:群公告"`

	// MemberCnt 群成员数量
	MemberCnt int `gorm:"column:member_cnt;default:1;comment:群人数"`

	// OwnerId 群主用户 UUID
	OwnerId string `gorm:"column:owner_id;uniqueIndex:idx_owner_name;type:char(20);not null;comment:群主uuid"`

	// AddMode 加群方式 (0=直接加入, 1=需要审核)
	AddMode int8 `gorm:"column:add_mode;default:0;comment:加群方式，0.直接，1.审核"`

	// Avatar 群头像 URL
	Avatar string `gorm:"column:avatar;type:char(255);default:https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png;not null;comment:头像"`

	// Status 群状态 (0=正常, 1=禁用, 2=解散)
	Status int8 `gorm:"column:status;default:0;comment:状态，0.正常，1.禁用，2.解散"`

	// Version 乐观锁版本号
	Version int32 `gorm:"column:version;default:0;not null;comment:乐观锁版本号"`
}

// TableName 指定表名
func (GroupInfo) TableName() string {
	return "group_info"
}

// GroupMember 群成员关联模型
// 对应数据库 group_member 表
// 刻意不包含 DeletedAt（禁用软删除）：成员退群/踢出直接物理删除释放唯一键 idx_group_user，历史变动审计由领域事件流承载
type GroupMember struct {
	ID        uint         `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time    `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time    `gorm:"column:updated_at;not null" json:"updated_at"`

	// GroupUuid 群组 UUID
	GroupUuid string `gorm:"type:char(20);index;not null;uniqueIndex:idx_group_user;comment:群组ID"`

	// UserUuid 用户 UUID
	UserUuid string `gorm:"type:char(20);index;not null;uniqueIndex:idx_group_user;comment:用户ID"`

	// Role 成员角色 (1=普通成员, 2=管理员, 3=群主)
	Role int8 `gorm:"default:1;comment:1普通成员 2管理员 3群主"`

	// MuteUntil 禁言截止时间
	MuteUntil sql.NullTime `gorm:"column:mute_until;comment:禁言截止时间"`
}

// TableName 指定表名
func (GroupMember) TableName() string {
	return "group_member"
}

// GroupMemberWithUserInfo 群成员详细信息（含用户资料）
type GroupMemberWithUserInfo struct {
	UserId   string `json:"userId"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// ==================== 请求与响应 DTO ====================

// CheckGroupAddModeRequest 检查群组加群模式请求
type CheckGroupAddModeRequest struct {
	GroupId string `json:"group_id" form:"group_id" binding:"required"`
}

// CreateGroupRequest 创建群聊请求
type CreateGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Notice      string `json:"notice"`
	AddMode     int8   `json:"add_mode"`
	Avatar      string `json:"avatar"`
	ClientToken string `json:"client_token"`
}

// DismissGroupRequest 解散群聊请求
type DismissGroupRequest struct {
	GroupId string `json:"group_id" binding:"required"`
}

// GetGroupInfoRequest 获取群组信息请求
type GetGroupInfoRequest struct {
	GroupId string `json:"group_id" form:"group_id" binding:"required"`
}

// GetGroupMemberListRequest 获取群成员列表请求
type GetGroupMemberListRequest struct {
	GroupId  string `json:"group_id" form:"group_id" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// LeaveGroupRequest 退出群聊请求
type LeaveGroupRequest struct {
	GroupId string `json:"group_id" binding:"required"`
}

// MuteMemberRequest 群成员禁言请求
type MuteMemberRequest struct {
	GroupId  string `json:"group_id" binding:"required"`
	UserId   string `json:"user_id" binding:"required"`
	Duration int    `json:"duration" binding:"min=0"`
}

// RemoveGroupMembersRequest 移除群成员请求
type RemoveGroupMembersRequest struct {
	GroupId  string   `json:"group_id" binding:"required"`
	UuidList []string `json:"uuid_list" binding:"required,min=1"`
}

// UpdateGroupInfoRequest 更新群聊信息请求
type UpdateGroupInfoRequest struct {
	Uuid    string  `json:"uuid" binding:"required"`
	Name    *string `json:"name"`
	Avatar  *string `json:"avatar"`
	AddMode *int8   `json:"add_mode"`
	Notice  *string `json:"notice"`
	Version *int32  `json:"version" binding:"required"`
}

// GetGroupInfoRespond 获取群聊信息响应
type GetGroupInfoRespond struct {
	Uuid      string `json:"uuid"`
	Name      string `json:"name"`
	Notice    string `json:"notice"`
	MemberCnt int    `json:"member_cnt"`
	OwnerId   string `json:"owner_id"`
	AddMode   int8   `json:"add_mode"`
	Status    int8   `json:"status"`
	Avatar    string `json:"avatar"`
	IsDeleted bool   `json:"is_deleted"`
	Version   int32  `json:"version"`
}

// GetGroupMemberListRespond 获取群成员列表响应
type GetGroupMemberListRespond struct {
	UserId   string `json:"user_id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// MyGroupListRespond 我的群组列表响应
type MyGroupListRespond struct {
	GroupId   string `json:"groupId"`
	GroupName string `json:"groupName"`
	Avatar    string `json:"avatar"`
}

// PublicGroupInfoRespond 公开群组信息响应
type PublicGroupInfoRespond struct {
	GroupId     string `json:"group_id"`
	GroupName   string `json:"group_name"`
	GroupAvatar string `json:"group_avatar"`
	GroupNotice string `json:"group_notice"`
	MemberCnt   int    `json:"member_cnt"`
	OwnerId     string `json:"owner_id"`
	AddMode     int8   `json:"add_mode"`
}
