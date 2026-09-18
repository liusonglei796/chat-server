// Package message 提供消息相关数据访问层的具体实现
// 本文件实现 MessageStore 接口，处理消息相关的数据库操作
package message

import (
	"context"
	"strconv"
	"strings"
	"time"

	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/errorx"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// messageStore MessageStore 接口的实现
type messageStore struct {
	db *gorm.DB // GORM 数据库实例
}

// NewMessageStore 创建 MessageStore 实例
// db: GORM 数据库实例
// 返回: MessageStore 接口实现
func NewMessageStore(db *gorm.DB) *messageStore {
	return &messageStore{db: db}
}

// FindByUserIdsPaged 根据两个用户ID查找私聊消息（分页）
// userOneId, userTwoId: 两个用户的 UUID
// page: 页码（从1开始）
// pageSize: 每页数量
// 返回: 消息列表、总数和错误
func (r *messageStore) FindByUserIdsPaged(ctx context.Context, userOneId, userTwoId string, page, pageSize int) ([]model.Message, int64, error) {
	var messages []model.Message
	var total int64

	// 校验分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	condition := "(send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)"

	// 统计总数
	if err := r.db.WithContext(ctx).Model(&model.Message{}).Where(condition,
		userOneId, userTwoId, userTwoId, userOneId).Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计私聊消息数量 user1=%s user2=%s", userOneId, userTwoId)
	}

	// 计算偏移量
	offset := (page - 1) * pageSize
	// 使用 OR 条件查找双向消息，按时间倒序排列（最新的在前）
	if err := r.db.WithContext(ctx).Where(condition,
		userOneId, userTwoId, userTwoId, userOneId).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&messages).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "查询消息 user1=%s user2=%s", userOneId, userTwoId)
	}
	return messages, total, nil
}

// FindByGroupIdPaged 根据群组ID分页查找群聊消息
// receiveId: 群组 UUID
// page: 页码（从1开始）
// pageSize: 每页数量
// 返回: 消息列表、总数和错误
func (r *messageStore) FindByGroupIdPaged(ctx context.Context, receiveId string, page, pageSize int) ([]model.Message, int64, error) {
	var messages []model.Message
	var total int64

	// 校验分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 统计总数
	if err := r.db.WithContext(ctx).Model(&model.Message{}).Where("receive_id = ?", receiveId).Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计群消息数量 receive_id=%s", receiveId)
	}

	// 计算偏移量并分页查询，按时间倒序（最新的在前）
	offset := (page - 1) * pageSize
	if err := r.db.WithContext(ctx).Where("receive_id = ?", receiveId).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&messages).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询群消息 receive_id=%s", receiveId)
	}
	return messages, total, nil
}

// UpdateStatus 更新消息状态
// uuid: 消息唯一标识
// status: 新状态值
// 返回: 操作错误
func (r *messageStore) UpdateStatus(ctx context.Context, uuid string, status int8) error {
	if err := r.db.WithContext(ctx).Model(&model.Message{}).Where("uuid = ?", uuid).Update("status", status).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新消息状态 uuid=%s", uuid)
	}
	return nil
}

// Create 创建新消息
// message: 消息结构体
// 返回: 操作错误
// 注意：使用 Clauses 实现 ON DUPLICATE KEY UPDATE，当 client_msg_id 冲突时静默忽略
// 这是 Redis 宕机时的兜底防重复机制
func (r *messageStore) Create(ctx context.Context, message *model.Message) error {
	if err := r.db.WithContext(ctx).Clauses(
		clause.OnConflict{
			Columns:   []clause.Column{{Name: "client_msg_id"}},
			DoNothing: true,
		},
	).Create(message).Error; err != nil {
		return dberr.WrapDBError(err, "创建消息")
	}
	return nil
}

// FindByUuid 根据消息UUID查找消息
// uuid: 消息唯一标识
// 返回: 消息实体和错误
func (r *messageStore) FindByUuid(ctx context.Context, uuid string) (*model.Message, error) {
	var msg model.Message
	if err := r.db.WithContext(ctx).Where("uuid = ?", uuid).First(&msg).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errorx.New(errorx.CodeNotFound, "消息不存在")
		}
		return nil, dberr.WrapDBErrorf(err, "查找消息 uuid=%s", uuid)
	}
	return &msg, nil
}

// UpdateContent 更新消息内容和类型（用于撤回）
// uuid: 消息唯一标识
// content: 新内容
// msgType: 新消息类型
// 返回: 操作错误
func (r *messageStore) UpdateContent(ctx context.Context, uuid, content string, msgType int8) error {
	if err := r.db.WithContext(ctx).Model(&model.Message{}).Where("uuid = ?", uuid).
		Updates(map[string]interface{}{
			"content": content,
			"type":    msgType,
		}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新消息内容 uuid=%s", uuid)
	}
	return nil
}

// applyMessageCursor 统一解析游标并拼装过滤及排序条件
// 优先使用单调自增的雪花算法 Uuid，同时兼容历史时间戳游标
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

// resolveNextCursor 获取下一页游标，优先返回雪花 ID 杜绝秒级精度碰撞
func resolveNextCursor(messages []model.Message) string {
	if len(messages) == 0 {
		return ""
	}
	lastMsg := messages[len(messages)-1]
	if lastMsg.Uuid != "" {
		return lastMsg.Uuid
	}
	return strconv.FormatInt(lastMsg.CreatedAt.Unix(), 10)
}

// FindByUserIdsCursor 根据两个用户ID查找私聊消息（游标分页）
// userOneId, userTwoId: 两个用户的 UUID
// cursor: 游标（雪花ID或Unix时间戳字符串）
// pageSize: 每页数量
// 返回: 消息列表、下一页游标、是否有更多数据、错误
func (r *messageStore) FindByUserIdsCursor(ctx context.Context, userOneId, userTwoId, cursor string, pageSize int) (*model.CursorPageMessageResult, error) {
	var messages []model.Message

	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	condition := "(send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)"
	query := r.db.WithContext(ctx).Where(condition, userOneId, userTwoId, userTwoId, userOneId)
	query = applyMessageCursor(query, cursor)

	// 多查一条用于判断是否有更多
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

	return &model.CursorPageMessageResult{
		Messages:   messages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// FindByGroupIdCursor 根据群组ID分页查找群聊消息（游标分页）
// 命中 idx_receive_uuid (receive_id, uuid DESC) 联合索引，消除 filesort
// receiveId: 群组 UUID
// cursor: 游标（雪花ID或Unix时间戳字符串）
// pageSize: 每页数量
// 返回: 消息列表、下一页游标、是否有更多数据、错误
func (r *messageStore) FindByGroupIdCursor(ctx context.Context, receiveId, cursor string, pageSize int) (*model.CursorPageMessageResult, error) {
	var messages []model.Message

	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).Where("receive_id = ?", receiveId)
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

	return &model.CursorPageMessageResult{
		Messages:   messages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// FindBySessionIdCursor 根据会话ID分页查找聊天消息（游标分页）
// 命中 idx_session_uuid (session_id, uuid DESC) 联合索引，消除 filesort
// sessionId: 会话 UUID
// cursor: 游标（雪花ID或Unix时间戳字符串）
// pageSize: 每页数量
// 返回: 消息列表、下一页游标、是否有更多数据、错误
func (r *messageStore) FindBySessionIdCursor(ctx context.Context, sessionId, cursor string, pageSize int) (*model.CursorPageMessageResult, error) {
	var messages []model.Message

	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).Where("session_id = ?", sessionId)
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

	return &model.CursorPageMessageResult{
		Messages:   messages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
