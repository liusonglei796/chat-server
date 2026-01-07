// Package model 定义数据库实体模型
// 本文件定义消息模型，用于存储聊天消息
package model

import (
	"database/sql"

	"gorm.io/gorm"
)

// Message 消息模型
// 对应数据库 message 表
// 存储单聊、群聊、音视频通话等所有类型的消息
type Message struct {
	gorm.Model

	// Uuid 消息唯一标识
	// 使用雪花算法生成的 int64 类型 ID
	// bigint 类型支持大数值，避免 ID 溢出
	Uuid int64 `gorm:"column:uuid;uniqueIndex;type:bigint;not null;comment:消息雪花ID"`

	// SessionId 会话 UUID
	// 关联到 Session 表，标识消息属于哪个会话
	SessionId string `gorm:"column:session_id;index;type:char(20);not null;comment:会话uuid"`

	// Type 消息类型
	// 0=文本消息, 1=语音消息, 2=文件消息, 3=音视频通话信令
	// 参见 pkg/enum/message_type_enum.go
	Type int8 `gorm:"column:type;not null;comment:消息类型，0.文本，1.语音，2.文件，3.通话"`

	// Content 消息文本内容
	// 对于文本消息存储实际内容，其他类型可能为空
	Content string `gorm:"column:content;type:TEXT;comment:消息内容"`

	// Url 资源 URL
	// 用于语音、文件等需要链接的消息类型
	//如果是图片、语音、视频、文件消息：这些多媒体文件通常不会直接存进数据库，而是先上传到对象存储（如阿里云 OSS、华为云 OBS 等）
	// ，然后把生成的 访问链接（URL） 存到这个 Url 字段里。
	Url string `gorm:"column:url;type:char(255);comment:消息url"`

	// SendId 发送者 UUID
	// 关联到 UserInfo 表
	SendId string `gorm:"column:send_id;index;type:char(20);not null;comment:发送者uuid"`

	// SendName 发送者昵称
	// 冗余存储，避免每次查询消息时都要关联用户表
	SendName string `gorm:"column:send_name;type:varchar(20);not null;comment:发送者昵称"`

	// SendAvatar 发送者头像
	// 冗余存储，存储相对路径如 "/static/avatars/xxx.jpg"
	SendAvatar string `gorm:"column:send_avatar;type:varchar(255);not null;comment:发送者头像"`

	// ReceiveId 接收者 UUID
	// 单聊时为用户 UUID（U开头），群聊时为群组 UUID（G开头）
	ReceiveId string `gorm:"column:receive_id;index;type:char(20);not null;comment:接受者uuid"`

	// FileType 文件 MIME 类型
	// 如 "image/jpeg", "application/pdf"
	FileType string `gorm:"column:file_type;type:char(50);comment:文件类型"`

	// FileName 文件名
	FileName string `gorm:"column:file_name;type:varchar(50);comment:文件名"`

	// FileSize 文件大小
	// 字符串格式，如 "1.5MB"
	FileSize string `gorm:"column:file_size;type:char(20);comment:文件大小"`

	// Status 消息状态
	// 0=未发送, 1=已发送
	// 参见 pkg/enum/message_status_enum.go
	Status int8 `gorm:"column:status;not null;comment:状态，0.未发送，1.已发送"`

	// SendAt 实际发送时间
	// 可能与 CreatedAt 不同（如离线消息）
	SendAt sql.NullTime `gorm:"column:send_at;comment:发送时间"`

	// AVdata 音视频通话信令数据
	// JSON 格式，包含 WebRTC 信令信息
	// 如 {"messageId":"PROXY","type":"start_call"}
	AVdata string `gorm:"column:av_data;comment:通话传递数据"`
}

// TableName 指定表名
func (Message) TableName() string {
	return "message"
}
