package message

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/pkg/errorx"
)

// ==================== 接口定义 ====================

// MessageStore 消息数据访问接口
type MessageStore interface {
	FindByUserIdsPaged(ctx context.Context, userOneId, userTwoId string, page, pageSize int) ([]Message, int64, error)
	FindByUserIdsCursor(ctx context.Context, userOneId, userTwoId, cursor string, pageSize int) (*CursorPageMessageResult, error)
	FindByGroupIdPaged(ctx context.Context, groupId string, page, pageSize int) ([]Message, int64, error)
	FindByGroupIdCursor(ctx context.Context, groupId, cursor string, pageSize int) (*CursorPageMessageResult, error)
	FindBySessionIdCursor(ctx context.Context, sessionId, cursor string, pageSize int) (*CursorPageMessageResult, error)
	FindBySessionIdSync(ctx context.Context, sessionId string, afterSeq int64, limit int) ([]Message, error)
	GetMaxSeqBySessionId(ctx context.Context, sessionId string) (int64, error)
	FindByUuid(ctx context.Context, uuid string) (*Message, error)
	UpdateContent(ctx context.Context, uuid, content string, msgType int8) (bool, error)
	Create(ctx context.Context, message *Message) error
}

// SessionStore 会话数据访问接口
type SessionStore interface {
	FindByUuid(ctx context.Context, uuid string) (*Session, error)
	FindBySendIdAndReceiveId(ctx context.Context, sendId, receiveId string) (*Session, error)
	FindBySendIdAndReceiveIdUnscoped(ctx context.Context, sendId, receiveId string) (*Session, error)
	FindBySendIdAndTypePaged(ctx context.Context, sendId string, receiveIdPrefix string, page, pageSize int) ([]Session, int64, error)
	FindBySendIdAndTypeCursor(ctx context.Context, sendId string, receiveIdPrefix, cursor string, pageSize int) (*CursorPageSessionResult, error)
	CreateSession(ctx context.Context, session *Session) error
	SoftDeleteByUuids(ctx context.Context, uuids []string) error
	SoftDeleteByUsers(ctx context.Context, userUuids []string) error
	UpdatePinStatus(ctx context.Context, uuid string, isPinned bool) error
	UpdateByReceiveId(ctx context.Context, receiveId string, updates map[string]interface{}) error
	UpdateLastMessage(ctx context.Context, sendId, receiveId, content string, msgType int8, msgTime time.Time, seqId int64) error
}

// ==================== MessageStore 实现 ====================

type messageStoreImpl struct {
	db *gorm.DB
}

// NewMessageStore 创建 MessageStore 实例
func NewMessageStore(db *gorm.DB) MessageStore {
	return &messageStoreImpl{db: db}
}

func (r *messageStoreImpl) FindByUserIdsPaged(ctx context.Context, userOneId, userTwoId string, page, pageSize int) ([]Message, int64, error) {
	var messages []Message
	var total int64

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	condition := "(send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)"

	if err := database.GetDB(ctx, r.db).Model(&Message{}).Where(condition,
		userOneId, userTwoId, userTwoId, userOneId).Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计私聊消息数量 user1=%s user2=%s", userOneId, userTwoId)
	}

	offset := (page - 1) * pageSize
	if err := database.GetDB(ctx, r.db).Where(condition,
		userOneId, userTwoId, userTwoId, userOneId).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&messages).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "查询消息 user1=%s user2=%s", userOneId, userTwoId)
	}
	return messages, total, nil
}

func (r *messageStoreImpl) FindByGroupIdPaged(ctx context.Context, receiveId string, page, pageSize int) ([]Message, int64, error) {
	var messages []Message
	var total int64

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	if err := database.GetDB(ctx, r.db).Model(&Message{}).Where("receive_id = ?", receiveId).Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计群消息数量 receive_id=%s", receiveId)
	}

	offset := (page - 1) * pageSize
	if err := database.GetDB(ctx, r.db).Where("receive_id = ?", receiveId).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&messages).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询群消息 receive_id=%s", receiveId)
	}
	return messages, total, nil
}

func (r *messageStoreImpl) Create(ctx context.Context, message *Message) error {
	if err := database.GetDB(ctx, r.db).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "client_msg_id"}},
			DoNothing: true,
		},
	).Create(message).Error; err != nil {
		return dberr.WrapDBError(err, "创建消息")
	}
	return nil
}

func (r *messageStoreImpl) FindByUuid(ctx context.Context, uuid string) (*Message, error) {
	var msg Message
	if err := database.GetDB(ctx, r.db).Where("uuid = ?", uuid).First(&msg).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errorx.New(errorx.CodeNotFound, "消息不存在")
		}
		return nil, dberr.WrapDBErrorf(err, "查找消息 uuid=%s", uuid)
	}
	return &msg, nil
}

func (r *messageStoreImpl) UpdateContent(ctx context.Context, uuid, content string, msgType int8) (bool, error) {
	result := database.GetDB(ctx, r.db).Model(&Message{}).
		Where("uuid = ? AND type != ?", uuid, msgType).
		Updates(map[string]interface{}{
			"content": content,
			"type":    msgType,
		})
	if result.Error != nil {
		return false, dberr.WrapDBErrorf(result.Error, "更新消息内容 uuid=%s", uuid)
	}
	return result.RowsAffected > 0, nil
}

func applyMessageCursor(query *gorm.DB, cursor string) *gorm.DB {
	if cursor == "" {
		return query.Order("uuid DESC")
	}
	if strings.HasPrefix(cursor, "M") {
		return query.Where("uuid < ?", cursor).Order("uuid DESC")
	}
	if timestamp, err := strconv.ParseInt(cursor, 10, 64); err == nil {
		cursorTime := time.Unix(timestamp, 0)
		return query.Where("created_at < ?", cursorTime).Order("created_at DESC, uuid DESC")
	}
	return query.Where("uuid < ?", cursor).Order("uuid DESC")
}

func resolveNextCursor(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}
	lastMsg := messages[len(messages)-1]
	if lastMsg.Uuid != "" {
		return lastMsg.Uuid
	}
	return strconv.FormatInt(lastMsg.CreatedAt.Unix(), 10)
}

func (r *messageStoreImpl) FindByUserIdsCursor(ctx context.Context, userOneId, userTwoId, cursor string, pageSize int) (*CursorPageMessageResult, error) {
	var messages []Message

	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	condition := "(send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)"
	query := database.GetDB(ctx, r.db).Where(condition, userOneId, userTwoId, userTwoId, userOneId)
	query = applyMessageCursor(query, cursor)

	if err := query.Limit(pageSize + 1).Find(&messages).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "游标分页查询私聊消息 user1=%s user2=%s", userOneId, userTwoId)
	}

	hasMore := len(messages) > pageSize
	if hasMore {
		messages = messages[:pageSize]
	}

	var nextCursor string
	if hasMore {
		nextCursor = resolveNextCursor(messages)
	}

	return &CursorPageMessageResult{
		Messages:   messages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (r *messageStoreImpl) FindByGroupIdCursor(ctx context.Context, receiveId, cursor string, pageSize int) (*CursorPageMessageResult, error) {
	var messages []Message

	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := database.GetDB(ctx, r.db).Where("receive_id = ?", receiveId)
	query = applyMessageCursor(query, cursor)

	if err := query.Limit(pageSize + 1).Find(&messages).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "游标分页查询群消息 receive_id=%s", receiveId)
	}

	hasMore := len(messages) > pageSize
	if hasMore {
		messages = messages[:pageSize]
	}

	var nextCursor string
	if hasMore {
		nextCursor = resolveNextCursor(messages)
	}

	return &CursorPageMessageResult{
		Messages:   messages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (r *messageStoreImpl) FindBySessionIdCursor(ctx context.Context, sessionId, cursor string, pageSize int) (*CursorPageMessageResult, error) {
	var messages []Message

	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := database.GetDB(ctx, r.db).Where("session_id = ?", sessionId)
	query = applyMessageCursor(query, cursor)

	if err := query.Limit(pageSize + 1).Find(&messages).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "游标分页查询会话消息 session_id=%s", sessionId)
	}

	hasMore := len(messages) > pageSize
	if hasMore {
		messages = messages[:pageSize]
	}

	var nextCursor string
	if hasMore {
		nextCursor = resolveNextCursor(messages)
	}

	return &CursorPageMessageResult{
		Messages:   messages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (r *messageStoreImpl) FindBySessionIdSync(ctx context.Context, sessionId string, afterSeq int64, limit int) ([]Message, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var messages []Message
	if err := database.GetDB(ctx, r.db).
		Where("session_id = ? AND seq_id > ?", sessionId, afterSeq).
		Order("seq_id ASC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "基于Seq增量同步消息 session_id=%s after_seq=%d", sessionId, afterSeq)
	}
	return messages, nil
}

func (r *messageStoreImpl) GetMaxSeqBySessionId(ctx context.Context, sessionId string) (int64, error) {
	var maxSeq int64
	if err := database.GetDB(ctx, r.db).Model(&Message{}).
		Where("session_id = ?", sessionId).
		Select("COALESCE(MAX(seq_id), 0)").
		Scan(&maxSeq).Error; err != nil {
		return 0, dberr.WrapDBErrorf(err, "查询会话最大seq_id session_id=%s", sessionId)
	}
	return maxSeq, nil
}

// ==================== SessionStore 实现 ====================

type sessionStoreImpl struct {
	db *gorm.DB
}

// NewSessionStore 创建 SessionStore 实例
func NewSessionStore(db *gorm.DB) SessionStore {
	return &sessionStoreImpl{db: db}
}

func (r *sessionStoreImpl) FindByUuid(ctx context.Context, uuid string) (*Session, error) {
	var session Session
	if err := database.GetDB(ctx, r.db).Where("uuid = ?", uuid).First(&session).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询会话 uuid=%s", uuid)
	}
	return &session, nil
}

func (r *sessionStoreImpl) FindBySendIdAndReceiveId(ctx context.Context, sendId, receiveId string) (*Session, error) {
	var session Session
	if err := database.GetDB(ctx, r.db).Where("send_id = ? AND receive_id = ?", sendId, receiveId).First(&session).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询会话 send_id=%s receive_id=%s", sendId, receiveId)
	}
	return &session, nil
}

func (r *sessionStoreImpl) FindBySendIdAndReceiveIdUnscoped(ctx context.Context, sendId, receiveId string) (*Session, error) {
	var session Session
	if err := database.GetDB(ctx, r.db).Where("send_id = ? AND receive_id = ?", sendId, receiveId).First(&session).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询会话 send_id=%s receive_id=%s", sendId, receiveId)
	}
	return &session, nil
}

func (r *sessionStoreImpl) FindBySendIdAndTypePaged(ctx context.Context, sendId string, receiveIdPrefix string, page, pageSize int) ([]Session, int64, error) {
	var sessions []Session
	var total int64

	condition := database.GetDB(ctx, r.db).Model(&Session{}).Where("send_id = ? AND receive_id LIKE ?", sendId, receiveIdPrefix+"%")

	if err := condition.Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计会话数量 send_id=%s type=%s", sendId, receiveIdPrefix)
	}

	offset := (page - 1) * pageSize
	if err := database.GetDB(ctx, r.db).Where("send_id = ? AND receive_id LIKE ?", sendId, receiveIdPrefix+"%").
		Order("is_pinned DESC, last_message_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&sessions).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询会话 send_id=%s type=%s", sendId, receiveIdPrefix)
	}
	return sessions, total, nil
}

func encodeSessionCursor(s Session) string {
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
	if ts, err := strconv.ParseInt(cursor, 10, 64); err == nil {
		return false, ts, true
	}
	return false, 0, false
}

func (r *sessionStoreImpl) FindBySendIdAndTypeCursor(ctx context.Context, sendId string, receiveIdPrefix, cursor string, pageSize int) (*CursorPageSessionResult, error) {
	var sessions []Session

	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := database.GetDB(ctx, r.db).Where("send_id = ? AND receive_id LIKE ?", sendId, receiveIdPrefix+"%")

	if cursor != "" {
		isPinned, ts, ok := parseSessionCursor(cursor)
		if !ok {
			zap.L().Warn("parse session cursor failed, ignore cursor", zap.String("cursor", cursor))
		} else {
			cursorTime := time.Unix(ts, 0)
			if isPinned {
				query = query.Where("(is_pinned = ? AND (last_message_at < ? OR (last_message_at IS NULL AND created_at < ?))) OR is_pinned = ?",
					true, cursorTime, cursorTime, false)
			} else {
				query = query.Where("is_pinned = ? AND (last_message_at < ? OR (last_message_at IS NULL AND created_at < ?))",
					false, cursorTime, cursorTime)
			}
		}
	}

	if err := query.
		Order("is_pinned DESC, last_message_at DESC, created_at DESC").
		Limit(pageSize + 1).
		Find(&sessions).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "游标分页查询会话 send_id=%s type=%s", sendId, receiveIdPrefix)
	}

	hasMore := len(sessions) > pageSize
	if hasMore {
		sessions = sessions[:pageSize]
	}

	var nextCursor string
	if len(sessions) > 0 && hasMore {
		lastSession := sessions[len(sessions)-1]
		nextCursor = encodeSessionCursor(lastSession)
	}

	return &CursorPageSessionResult{
		Sessions:   sessions,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (r *sessionStoreImpl) CreateSession(ctx context.Context, session *Session) error {
	if err := database.GetDB(ctx, r.db).Create(session).Error; err != nil {
		if dberr.IsDuplicateKey(err) {
			var existing Session
			if queryErr := database.GetDB(ctx, r.db).Where("send_id = ? AND receive_id = ?", session.SendId, session.ReceiveId).First(&existing).Error; queryErr == nil {
				session.Uuid = existing.Uuid
				return nil
			}
		}
		return dberr.WrapDBError(err, "创建会话")
	}
	return nil
}

func (r *sessionStoreImpl) SoftDeleteByUuids(ctx context.Context, uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := database.GetDB(ctx, r.db).Where("uuid IN ?", uuids).Delete(&Session{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除会话")
	}
	return nil
}

func (r *sessionStoreImpl) SoftDeleteByUsers(ctx context.Context, userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := database.GetDB(ctx, r.db).Where("send_id IN ? OR receive_id IN ?", userUuids, userUuids).Delete(&Session{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除会话")
	}
	return nil
}

func (r *sessionStoreImpl) UpdateByReceiveId(ctx context.Context, receiveId string, updates map[string]interface{}) error {
	if err := database.GetDB(ctx, r.db).Model(&Session{}).Where("receive_id = ?", receiveId).Updates(updates).Error; err != nil {
		return dberr.WrapDBErrorf(err, "批量更新会话 receive_id=%s", receiveId)
	}
	return nil
}

func (r *sessionStoreImpl) UpdateLastMessage(ctx context.Context, sendId, receiveId, content string, msgType int8, msgTime time.Time, seqId int64) error {
	updates := map[string]interface{}{
		"last_message":      content,
		"last_message_at":   msgTime,
		"last_message_type": msgType,
	}
	if seqId > 0 {
		updates["last_seq_id"] = seqId
	}

	query := database.GetDB(ctx, r.db).Model(&Session{}).Where("send_id = ? AND receive_id = ?", sendId, receiveId)
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

func (r *sessionStoreImpl) UpdatePinStatus(ctx context.Context, uuid string, isPinned bool) error {
	if err := database.GetDB(ctx, r.db).Model(&Session{}).
		Where("uuid = ? AND is_pinned != ?", uuid, isPinned).
		Update("is_pinned", isPinned).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新会话置顶状态 uuid=%s", uuid)
	}
	return nil
}
