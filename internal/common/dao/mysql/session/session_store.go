// Package session 提供会话相关数据访问层的具体实现
// 本文件实现 SessionStore 接口，处理会话相关的数据库操作
package session

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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

// FindBySendIdAndReceiveIdUnscoped 包含软删除记录查找会话（用于唯一键冲突后的兜底回显与复活）
func (r *sessionStore) FindBySendIdAndReceiveIdUnscoped(ctx context.Context, sendId, receiveId string) (*model.Session, error) {
	var session model.Session
	if err := r.db.WithContext(ctx).Unscoped().Where("send_id = ? AND receive_id = ?", sendId, receiveId).First(&session).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询会话(含软删除) send_id=%s receive_id=%s", sendId, receiveId)
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

// encodeSessionCursor 生成置顶感知的下一页游标格式: <is_pinned>_<unix_time>
func encodeSessionCursor(s model.Session) string {
	pinVal := 0
	if s.IsPinned {
		pinVal = 1
	}
	var ts int64
	if s.LastMessageAt.Valid {
		ts = s.LastMessageAt.Time.Unix()
	} else {
		ts = s.CreatedAt.Unix()
	}
	return fmt.Sprintf("%d_%d", pinVal, ts)
}

// parseSessionCursor 解析游标字符串，返回 (isPinned, timestamp, ok)
// 支持新格式 "<is_pinned>_<timestamp>"，同时兼容历史纯时间戳数字 "<timestamp>"
func parseSessionCursor(cursor string) (isPinned bool, timestamp int64, ok bool) {
	if cursor == "" {
		return false, 0, false
	}
	if idx := strings.IndexByte(cursor, '_'); idx != -1 {
		pinVal, err1 := strconv.Atoi(cursor[:idx])
		ts, err2 := strconv.ParseInt(cursor[idx+1:], 10, 64)
		if err1 == nil && err2 == nil {
			return pinVal == 1, ts, true
		}
	} else if idx := strings.IndexByte(cursor, ':'); idx != -1 {
		pinVal, err1 := strconv.Atoi(cursor[:idx])
		ts, err2 := strconv.ParseInt(cursor[idx+1:], 10, 64)
		if err1 == nil && err2 == nil {
			return pinVal == 1, ts, true
		}
	}
	// 兼容旧格式：纯时间戳数字（视为未置顶会话时间戳）
	if ts, err := strconv.ParseInt(cursor, 10, 64); err == nil {
		return false, ts, true
	}
	return false, 0, false
}

// FindBySendIdAndTypeCursor 根据发送者ID和接收者类型前缀游标分页查找会话
// receiveIdPrefix: "U" 表示私聊会话，"G" 表示群聊会话
// cursor: 游标（格式: "<is_pinned>_<unix_time>" 或纯时间戳字符串）
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

	// 构建基础查询
	query := r.db.WithContext(ctx).Where("send_id = ? AND receive_id LIKE ?", sendId, receiveIdPrefix+"%")

	// 如果有游标，基于置顶状态与时间戳进行双维度下钻，避免跨页丢弃普通会话
	if cursor != "" {
		isPinned, ts, ok := parseSessionCursor(cursor)
		if !ok {
			zap.L().Warn("parse session cursor failed, ignore cursor", zap.String("cursor", cursor))
		} else {
			cursorTime := time.Unix(ts, 0)
			if isPinned {
				// 游标仍在置顶会话段：后续包含比当前置顶会话更早的置顶会话，或者进入未置顶会话段
				query = query.Where("(is_pinned = ? AND (last_message_at < ? OR (last_message_at IS NULL AND created_at < ?))) OR is_pinned = ?",
					true, cursorTime, cursorTime, false)
			} else {
				// 游标已进入未置顶会话段：仅查询比当前未置顶会话更早的未置顶会话
				query = query.Where("is_pinned = ? AND (last_message_at < ? OR (last_message_at IS NULL AND created_at < ?))",
					false, cursorTime, cursorTime)
			}
		}
	}

	// 先按置顶状态倒序，再按最后消息时间倒序，最后按创建时间倒序消除不确定性
	// 多查一条用于判断是否有更多
	if err := query.
		Order("is_pinned DESC, last_message_at DESC, created_at DESC").
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
		nextCursor = encodeSessionCursor(lastSession)
	}

	return &model.CursorPageSessionResult{
		Sessions:   sessions,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// CreateSession 创建会话
// 当 (send_id, receive_id) 唯一键冲突时返回 DuplicateKey 错误，由业务层捕获并返回已有会话 UUID。
// 若发现冲突记录已被软删除，自动清除 deleted_at 复活该会话，彻底消除唯一索引与软删除的死锁
func (r *sessionStore) CreateSession(ctx context.Context, session *model.Session) error {
	if err := r.db.WithContext(ctx).Create(session).Error; err != nil {
		if dberr.IsDuplicateKey(err) {
			var existing model.Session
			if queryErr := r.db.WithContext(ctx).Unscoped().Where("send_id = ? AND receive_id = ?", session.SendId, session.ReceiveId).First(&existing).Error; queryErr == nil {
				if existing.DeletedAt.Valid {
					// 软删除复活：重置 deleted_at 状态为活跃并更新时间戳
					_ = r.db.WithContext(ctx).Unscoped().Model(&existing).Update("deleted_at", nil)
					session.Uuid = existing.Uuid
				}
			}
		}
		return dberr.WrapDBError(err, "创建会话")
	}
	return nil
}

// SoftDeleteByUuids 批量删除会话（物理删除，释放唯一索引槽位避免再次建立会话冲突）
func (r *sessionStore) SoftDeleteByUuids(ctx context.Context, uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Unscoped().Where("uuid IN ?", uuids).Delete(&model.Session{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除会话")
	}
	return nil
}

// SoftDeleteByUsers 批量删除指定用户的所有会话（物理删除，释放唯一索引槽位）
// 删除用户发起的和接收的所有会话
func (r *sessionStore) SoftDeleteByUsers(ctx context.Context, userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Unscoped().Where("send_id IN ? OR receive_id IN ?", userUuids, userUuids).Delete(&model.Session{}).Error; err != nil {
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
// 采用双重版本乐观锁：
//  1. 若 seqId > 0，采用严格单调递增的 last_seq_id 比对 (last_seq_id = 0 或 last_seq_id <= seqId)，彻底杜绝分布式时钟漂移与网络乱序覆盖；
//  2. 若 seqId <= 0（兼容旧数据场景），退避使用 last_message_at 物理时间戳比对
func (r *sessionStore) UpdateLastMessage(ctx context.Context, sendId, receiveId, content string, msgType int8, msgTime time.Time, seqId int64) error {
	updates := map[string]interface{}{
		"last_message":      content,
		"last_message_at":   msgTime,
		"last_message_type": msgType,
	}
	if seqId > 0 {
		updates["last_seq_id"] = seqId
	}

	query := r.db.WithContext(ctx).Model(&model.Session{}).Where("send_id = ? AND receive_id = ?", sendId, receiveId)
	if seqId > 0 {
		query = query.Where("(last_seq_id = 0 OR last_seq_id <= ?)", seqId)
	} else {
		query = query.Where("(last_message_at IS NULL OR last_message_at <= ?)", msgTime)
	}

	if err := query.Updates(updates).Error; err != nil {
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
