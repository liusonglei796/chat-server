package friendship

import (
	"time"
)

// Friendship 好友关系模型
// 对应数据库 user_friendship 表
// 仅存储用户之间的好友关系（群组关系由 GroupMember 表管理）
// user_friendship 是已建立的好友关系表；apply 是申请阶段的记录表（好友/入群申请）。
// 刻意不包含 DeletedAt（禁用软删除）：好友关系属于瞬态映射槽位，解除好友直接物理删除释放唯一键 idx_user_friend
type Friendship struct {
	// UserId 用户 UUID - 联合主键
	UserId string `gorm:"column:user_id;primaryKey;type:char(20);not null;comment:用户唯一id" json:"user_id"`

	// FriendId 好友用户 UUID - 联合主键
	FriendId string `gorm:"column:friend_id;primaryKey;type:char(20);not null;comment:好友ID" json:"friend_id"`

	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`

	// Status 好友状态
	// 0=正常：正常好友关系
	// 1=拉黑：我拉黑了对方
	// 2=被拉黑：被对方拉黑
	Status int8 `gorm:"column:status;not null;comment:好友状态，0.正常，1.拉黑，2.被拉黑"`

	// Remark 好友备注名
	// 用户对好友设置的自定义备注名称
	Remark string `gorm:"column:remark;type:varchar(50);default:'';comment:好友备注"`

	// Version 乐观锁版本号
	// 用于循环状态机跃迁防 ABA 与迟到请求覆盖
	Version int64 `gorm:"column:version;default:1;not null;comment:乐观锁版本号"`
}

// TableName 指定表名
func (Friendship) TableName() string {
	return "user_friendship"
}

// FriendInfo 好友详细信息响应数据
type FriendInfo struct {
	FriendId        string `json:"friend_id"`
	FriendName      string `json:"friend_name"`
	FriendAvatar    string `json:"friend_avatar"`
	FriendBirthday  string `json:"friend_birthday"`
	FriendEmail     string `json:"friend_email"`
	FriendPhone     string `json:"friend_phone"`
	FriendGender    int8   `json:"friend_gender"`
	FriendSignature string `json:"friend_signature"`
	Remark          string `json:"remark"`
}

// FriendItem 好友列表简略项响应数据
type FriendItem struct {
	UserId   string `json:"user_id"`
	UserName string `json:"user_name"`
	Avatar   string `json:"avatar"`
}

// ==================== 请求 DTO ====================

// BatchDeleteRequest 批量删除好友请求
type BatchDeleteRequest struct {
	UuidList []string `json:"uuid_list" binding:"required,min=1"`
}

// BlockFriendRequest 拉黑/取消拉黑好友请求
type BlockFriendRequest struct {
	FriendId string `json:"friend_id" binding:"required"`
}

// GetFriendInfoRequest 获取好友信息请求
type GetFriendInfoRequest struct {
	FriendId string `json:"friend_id" form:"friend_id" binding:"required"`
}

// GetFriendListRequest 获取好友列表请求
type GetFriendListRequest struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// GetJoinedGroupListRequest 获取已加入群组列表请求
type GetJoinedGroupListRequest struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// UpdateRemarkRequest 更新好友备注请求
type UpdateRemarkRequest struct {
	FriendId string `json:"friend_id" binding:"required"`
	Remark   string `json:"remark" binding:"max=50"`
}
