// Package session 提供会话相关数据访问层的具体实现
// 本文件实现 SessionStore 接口，处理会话相关的数据库操作
package session

import (
	"context"
	"strconv"
	"time"

	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/model"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// sessionStore SessionStore 接口的实现
type sessionStore struct {
	db *gorm.DB // GORM 数据库实例
}

// NewSessionStore 创建 SessionStore 实例
func NewSessionStore(db *gorm.DB) *sessionStore {
	return &sessionStore{db: db}
}

// FindByUuid 根据会话UUID查找会话
func (r *sessionStore) FindByUuid(ctx context.Context, uuid string) (*model.Session, error) {
	var session model.Session
	if err := r.db.WithContext(ctx).Where("uuid = ?", uuid).First(&session).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询会话 uuid=%s", uuid)
	}
	return &session, nil
}

// FindBySendIdAndReceiveId 根据发送者和接收者查找会话
// 用于查找两个实体之间是否已存在会话
func (r *sessionStore) FindBySendIdAndReceiveId(ctx context.Context, sendId, receiveId string) (*model.Session, error) {
	var session model.Session
	if err := r.db.WithContext(ctx).Where("send_id = ? AND receive_id = ?", sendId, receiveId).First(&session).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询会话 send_id=%s receive_id=%s", sendId, receiveId)
	}
	return &session, nil
}

// FindBySendIdAndTypePaged 根据发送者ID和接收者类型前缀分页查找会话
// receiveIdPrefix: "U" 表示私聊会话，"G" 表示群聊会话
//
// 排序规则：
//  1. 置顶会话优先显示（is_pinned = true 排在前面）
//  2. 置顶会话内部按最后消息时间倒序
//  3. 非置顶会话按最后消息时间倒序
func (r *sessionStore) FindBySendIdAndTypePaged(ctx context.Context, sendId string, receiveIdPrefix string, page, pageSize int) ([]model.Session, int64, error) {
	var sessions []model.Session
	var total int64

	condition := r.db.WithContext(ctx).Model(&model.Session{}).Where("send_id = ? AND receive_id LIKE ?", sendId, receiveIdPrefix+"%")

	// 统计总数
	if err := condition.Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计会话数量 send_id=%s type=%s", sendId, receiveIdPrefix)
	}

	// 分页查询：先按置顶状态倒序，再按最后消息时间倒序
	offset := (page - 1) * pageSize
	if err := r.db.WithContext(ctx).Where("send_id = ? AND receive_id LIKE ?", sendId, receiveIdPrefix+"%").
		Order("is_pinned DESC, last_message_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&sessions).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询会话 send_id=%s type=%s", sendId, receiveIdPrefix)
	}
	return sessions, total, nil
}

// FindBySendIdAndTypeCursor 根据发送者ID和接收者类型前缀游标分页查找会话
// receiveIdPrefix: "U" 表示私聊会话，"G" 表示群聊会话
// cursor: 游标时间戳（上一页最后一条会话的 last_message_at Unix 时间戳）
//
// 排序规则：
//  1. 置顶会话优先显示（is_pinned = true 排在前面）
//  2. 置顶会话内部按最后消息时间倒序
//  3. 非置顶会话按最后消息时间倒序
func (r *sessionStore) FindBySendIdAndTypeCursor(ctx context.Context, sendId string, receiveIdPrefix, cursor string, pageSize int) (*model.CursorPageSessionResult, error) {
	var sessions []model.Session

	// 校验分页参数
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 构建查询
	query := r.db.WithContext(ctx).Where("send_id = ? AND receive_id LIKE ?", sendId, receiveIdPrefix+"%")

	// 如果有游标，基于游标时间戳查询
	// 游标逻辑：查询 last_message_at < cursor 的数据
	if cursor != "" {
		timestamp, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil {
			// 解析失败则忽略游标，从最新开始查询
			zap.L().Warn("parse cursor failed, ignore cursor", zap.String("cursor", cursor), zap.Error(err))
		} else {
			cursorTime := time.Unix(timestamp, 0)
			query = query.Where("last_message_at < ?", cursorTime)
		}
	}

	// 先按置顶状态倒序，再按最后消息时间倒序
	// 多查一条用于判断是否有更多
	if err := query.
		Order("is_pinned DESC, last_message_at DESC").
		Limit(pageSize + 1).
		Find(&sessions).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "游标分页查询会话 send_id=%s type=%s", sendId, receiveIdPrefix)
	}

	// 判断是否有更多
	hasMore := len(sessions) > pageSize
	if hasMore {
		sessions = sessions[:pageSize] // 截取实际需要的数据
	}

	// 生成下一页游标
	var nextCursor string
	if len(sessions) > 0 && hasMore {
		lastSession := sessions[len(sessions)-1]
		if lastSession.LastMessageAt.Valid {
			nextCursor = strconv.FormatInt(lastSession.LastMessageAt.Time.Unix(), 10)
		}
	}

	return &model.CursorPageSessionResult{
		Sessions:   sessions,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// CreateSession 创建会话
func (r *sessionStore) CreateSession(ctx context.Context, session *model.Session) error {
	if err := r.db.WithContext(ctx).Create(session).Error; err != nil {
		return dberr.WrapDBError(err, "创建会话")
	}
	return nil
}

// SoftDeleteByUuids 批量软删除会话（按照会话ID）
func (r *sessionStore) SoftDeleteByUuids(ctx context.Context, uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Where("uuid IN ?", uuids).Delete(&model.Session{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除会话")
	}
	return nil
}

// SoftDeleteByUsers 批量软删除指定用户的所有会话
// 删除用户发起的和接收的所有会话
func (r *sessionStore) SoftDeleteByUsers(ctx context.Context, userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Where("send_id IN ? OR receive_id IN ?", userUuids, userUuids).Delete(&model.Session{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除会话")
	}
	return nil
}

// UpdateByReceiveId 根据接收者ID批量更新会话冗余字段
//
// 应用场景：
//  1. 用户修改昵称/头像时，同步更新所有与该用户的私聊会话
//  2. 群组修改名称/头像时，同步更新所有成员的群聊会话
//  3. 确保会话列表显示的信息与最新的用户/群组资料保持一致
//
// 说明：
//
//	Session表中冗余存储了receive_name和avatar字段，用于快速展示会话列表。
//	当用户/群组信息变更时，需要批量更新所有相关会话的冗余字段，避免显示旧信息。
//	这是典型的"写时同步"策略，以空间换时间，避免查询时的JOIN操作。
//
// 参数：
//   - receiveId: 接收者ID（可以是用户UUID或群组UUID）
//   - updates:   要更新的字段映射，如 {"receive_name": "新昵称", "avatar": "新头像URL"}
//
// 使用示例：
//
//	sessionUpdates := map[string]interface{}{
//	    "receive_name": "新群名",
//	    "avatar": "https://example.com/new-avatar.jpg",
//	}
//	err := sessionStore.UpdateByReceiveId("group-uuid", sessionUpdates)
func (r *sessionStore) UpdateByReceiveId(ctx context.Context, receiveId string, updates map[string]interface{}) error {
	if err := r.db.WithContext(ctx).Model(&model.Session{}).Where("receive_id = ?", receiveId).Updates(updates).Error; err != nil {
		return dberr.WrapDBErrorf(err, "批量更新会话 receive_id=%s", receiveId)
	}
	return nil
}

// UpdateLastMessage 更新会话的最后一条消息信息
// 用于发送消息后同步更新会话列表显示的最新消息
func (r *sessionStore) UpdateLastMessage(ctx context.Context, sendId, receiveId, content string, msgType int8, msgTime time.Time) error {
	updates := map[string]interface{}{
		"last_message":      content,
		"last_message_at":   msgTime,
		"last_message_type": msgType,
	}
	if err := r.db.WithContext(ctx).Model(&model.Session{}).
		Where("send_id = ? AND receive_id = ?", sendId, receiveId).
		Updates(updates).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新会话最后消息 send_id=%s receive_id=%s", sendId, receiveId)
	}
	return nil
}

// UpdatePinStatus 更新会话置顶状态
func (r *sessionStore) UpdatePinStatus(ctx context.Context, uuid string, isPinned bool) error {
	if err := r.db.WithContext(ctx).Model(&model.Session{}).Where("uuid = ?", uuid).
		Update("is_pinned", isPinned).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新会话置顶状态 uuid=%s", uuid)
	}
	return nil
}
