package message

import (
	"database/sql"
	"time"
)

// Message 消息模型
// 对应数据库 message 表
// 显式声明主键与时间戳，严禁使用 gorm.Model（消除隐式软删除与全局 WHERE deleted_at IS NULL 注入）
type Message struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`

	// Uuid 消息唯一标识 (M + SnowflakeID)
	Uuid string `gorm:"column:uuid;uniqueIndex;type:char(20);not null;index:idx_session_uuid,priority:2;index:idx_receive_uuid,priority:2;comment:消息雪花ID"`

	// ClientMsgId 客户端生成的消息唯一标识（幂等去重）
	ClientMsgId string `gorm:"column:client_msg_id;uniqueIndex;type:varchar(64);default:null;comment:客户端消息唯一标识"`

	// SessionId 会话 UUID
	SessionId string `gorm:"column:session_id;type:varchar(64);not null;index:idx_session_uuid,priority:1;index:idx_session_seq,priority:1;comment:会话uuid"`

	// SeqId 会话内单调递增序号
	SeqId int64 `gorm:"column:seq_id;not null;default:0;index:idx_session_seq,priority:2;comment:会话内单调递增序号"`

	// Type 消息类型 (0.文本 1.语音 2.文件 3.通话)
	Type int8 `gorm:"column:type;not null;comment:消息类型，0.文本，1.语音，2.文件，3.通话"`

	// Content 消息文本内容
	Content string `gorm:"column:content;type:TEXT;comment:消息内容"`

	// Url 多媒体资源 URL
	Url string `gorm:"column:url;type:char(255);comment:消息url"`

	// SendId 发送者 UUID
	SendId string `gorm:"column:send_id;index;type:char(20);not null;comment:发送者uuid"`

	// ReceiveId 接收者 UUID（用户或群组）
	ReceiveId string `gorm:"column:receive_id;index;type:char(20);not null;index:idx_receive_uuid,priority:1;comment:接受者uuid"`

	// FileType 文件 MIME 类型
	FileType string `gorm:"column:file_type;type:char(50);comment:文件类型"`

	// FileName 文件名
	FileName string `gorm:"column:file_name;type:varchar(50);comment:文件名"`

	// FileSize 文件大小
	FileSize string `gorm:"column:file_size;type:char(20);comment:文件大小"`

	// AVdata 音视频通话信令数据
	AVdata string `gorm:"column:av_data;comment:通话传递数据"`
}

// TableName 指定表名
func (Message) TableName() string {
	return "message"
}

// Session 会话模型
// 对应数据库 session 表
// 刻意不包含 DeletedAt（禁用软删除）：会话纯属客户端窗口展现状态，关闭/删除直接物理删除释放唯一键 idx_send_receive
type Session struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null" json:"updated_at"`

	// Uuid 会话唯一标识
	Uuid string `gorm:"column:uuid;uniqueIndex;type:char(20);comment:会话uuid"`

	// SendId 创建会话的用户 UUID
	SendId string `gorm:"column:send_id;index;type:char(20);not null;uniqueIndex:idx_send_receive;comment:创建会话人id"`

	// ReceiveId 接收会话的实体 UUID (用户或群组)
	ReceiveId string `gorm:"column:receive_id;index;type:char(20);not null;uniqueIndex:idx_send_receive;comment:接受会话人id"`

	// LastMessage 最新消息内容
	LastMessage string `gorm:"column:last_message;type:TEXT;comment:最新的消息"`

	// LastMessageAt 最后消息时间
	LastMessageAt sql.NullTime `gorm:"column:last_message_at;type:datetime;comment:最近接收时间"`

	// LastMessageType 最新消息类型
	LastMessageType int8 `gorm:"column:last_message_type;default:0;comment:最新消息类型"`

	// LastSeqId 会话内最新消息序列号
	LastSeqId int64 `gorm:"column:last_seq_id;default:0;comment:会话内最新消息序列号"`

	// IsPinned 是否置顶
	IsPinned bool `gorm:"column:is_pinned;default:false;comment:是否置顶"`
}

// TableName 指定表名
func (Session) TableName() string {
	return "session"
}

// CursorPageMessageResult 游标分页消息结果
type CursorPageMessageResult struct {
	Messages   []Message
	NextCursor string
	HasMore    bool
}

// CursorPageSessionResult 游标分页会话结果
type CursorPageSessionResult struct {
	Sessions   []Session
	NextCursor string
	HasMore    bool
}

// ==================== 消息 DTO ====================

// ChatMessageRequest 发送消息请求
type ChatMessageRequest struct {
	SessionId   string `json:"session_id"`
	Type        int8   `json:"type" binding:"required"`
	Content     string `json:"content"`
	Url         string `json:"url"`
	SendId      string `json:"send_id" binding:"required"`
	SendName    string `json:"send_name"`
	SendAvatar  string `json:"send_avatar"`
	ReceiveId   string `json:"receive_id" binding:"required"`
	FileSize    string `json:"file_size"`
	FileType    string `json:"file_type"`
	FileName    string `json:"file_name"`
	AVdata      string `json:"av_data"`
	ClientMsgId string `json:"client_msg_id"`
}

// AVSignalData 音视频通话信令数据
type AVSignalData struct {
	MessageId string `json:"messageId"`
	Type      string `json:"type"`
}

// GetMessageListRequest 获取消息列表请求
type GetMessageListRequest struct {
	TargetId string `form:"target_id" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Cursor   string `form:"cursor"`
}

// RecallMessageRequest 撤回消息请求
type RecallMessageRequest struct {
	MessageUuid string `json:"message_uuid" binding:"required"`
}

// AVMessageRespond 音视频信令响应
type AVMessageRespond struct {
	SendId     string `json:"send_id"`
	SendName   string `json:"send_name"`
	SendAvatar string `json:"send_avatar"`
	ReceiveId  string `json:"receive_id"`
	Type       int8   `json:"type"`
	Content    string `json:"content"`
	Url        string `json:"url"`
	FileType   string `json:"file_type"`
	FileName   string `json:"file_name"`
	FileSize   string `json:"file_size"`
	SeqId      int64  `json:"seq_id"`
	CreatedAt  string `json:"created_at"`
	AVdata     string `json:"av_data"`
}

// GetMessageListRespond 消息列表项响应
type GetMessageListRespond struct {
	SendId     string `json:"send_id"`
	SendName   string `json:"send_name"`
	SendAvatar string `json:"send_avatar"`
	ReceiveId  string `json:"receive_id"`
	Type       int8   `json:"type"`
	Content    string `json:"content"`
	Url        string `json:"url"`
	FileType   string `json:"file_type"`
	FileName   string `json:"file_name"`
	FileSize   string `json:"file_size"`
	SeqId      int64  `json:"seq_id"`
	CreatedAt  string `json:"created_at"`
}

// SendMessageRespond 发送消息成功响应
type SendMessageRespond struct {
	MessageUuid string `json:"message_uuid"`
	SeqId       int64  `json:"seq_id"`
	CreatedAt   string `json:"created_at"`
}

// ==================== 会话 DTO ====================

// CheckSessionAllowedRequest 检查会话权限请求
type CheckSessionAllowedRequest struct {
	ReceiveId string `json:"receive_id" form:"receive_id" binding:"required"`
}

// GetSessionListRequest 获取会话列表请求
type GetSessionListRequest struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Cursor   string `form:"cursor"`
}

// OpenSessionRequest 打开会话请求
type OpenSessionRequest struct {
	ReceiveId   string `json:"receive_id" binding:"required"`
	ClientToken string `json:"client_token"`
}

// PinSessionRequest 置顶会话请求
type PinSessionRequest struct {
	SessionId string `json:"session_id" form:"session_id"`
	Uuid      string `json:"uuid"`
	IsPinned  bool   `json:"is_pinned"`
}

// GroupSessionListRespond 群聊会话列表项响应
type GroupSessionListRespond struct {
	SessionId       string `json:"session_id"`
	GroupName       string `json:"group_name"`
	GroupId         string `json:"group_id"`
	Avatar          string `json:"avatar"`
	LastMessage     string `json:"last_message"`
	LastMessageTime string `json:"last_message_time"`
	LastMessageType int8   `json:"last_message_type"`
	LastSeqId       int64  `json:"last_seq_id"`
	IsPinned        bool   `json:"is_pinned"`
}

// UserSessionListRespond 单聊会话列表项响应
type UserSessionListRespond struct {
	SessionId       string `json:"session_id"`
	Avatar          string `json:"avatar"`
	UserId          string `json:"user_id"`
	Username        string `json:"user_name"`
	LastMessage     string `json:"last_message"`
	LastMessageTime string `json:"last_message_time"`
	LastMessageType int8   `json:"last_message_type"`
	LastSeqId       int64  `json:"last_seq_id"`
	IsPinned        bool   `json:"is_pinned"`
}
