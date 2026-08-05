// Package message 提供消息相关数据访问层的具体实现
// 本文件实现 MessageStore 接口，处理消息相关的数据库操作
package message

import (
	"context"
	"strconv"
	"time"

	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/errorx"

	"go.uber.org/zap"
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

// FindByUserIdsCursor 根据两个用户ID查找私聊消息（游标分页）
// userOneId, userTwoId: 两个用户的 UUID
// cursor: 游标时间戳（上一页最后一条消息的 created_at Unix 时间戳）
// pageSize: 每页数量
// 返回: 消息列表、下一页游标、是否有更多数据、错误
func (r *messageStore) FindByUserIdsCursor(ctx context.Context, userOneId, userTwoId, cursor string, pageSize int) (*model.CursorPageMessageResult, error) {
	var messages []model.Message

	// 校验分页参数
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	condition := "(send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)"

	// 构建查询
	query := r.db.WithContext(ctx).Where(condition, userOneId, userTwoId, userTwoId, userOneId)

	// 如果有游标，基于游标时间戳查询
	if cursor != "" {
		timestamp, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil {
			// 解析失败则忽略游标，从最新开始查询
			zap.L().Warn("parse cursor failed, ignore cursor", zap.String("cursor", cursor), zap.Error(err))
		} else {
			cursorTime := time.Unix(timestamp, 0)
			query = query.Where("created_at < ?", cursorTime)
		}
	}

	// 使用 OR 条件查找双向消息，按时间倒序排列（最新的在前）
	// 多查一条用于判断是否有更多
	if err := query.
		Order("created_at DESC").
		Limit(pageSize + 1).
		Find(&messages).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "游标分页查询私聊消息 user1=%s user2=%s", userOneId, userTwoId)
	}

	// 判断是否有更多
	hasMore := len(messages) > pageSize
	if hasMore {
		messages = messages[:pageSize] // 截取实际需要的数据
	}

	// 生成下一页游标
	var nextCursor string
	if len(messages) > 0 && hasMore {
		nextCursor = strconv.FormatInt(messages[len(messages)-1].CreatedAt.Unix(), 10)
	}

	return &model.CursorPageMessageResult{
		Messages:   messages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// FindByGroupIdCursor 根据群组ID分页查找群聊消息（游标分页）
// receiveId: 群组 UUID
// cursor: 游标时间戳（上一页最后一条消息的 created_at Unix 时间戳）
// pageSize: 每页数量
// 返回: 消息列表、下一页游标、是否有更多数据、错误
func (r *messageStore) FindByGroupIdCursor(ctx context.Context, receiveId, cursor string, pageSize int) (*model.CursorPageMessageResult, error) {
	var messages []model.Message

	// 校验分页参数
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 构建查询
	query := r.db.WithContext(ctx).Where("receive_id = ?", receiveId)

	// 如果有游标，基于游标时间戳查询
	if cursor != "" {
		timestamp, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil {
			// 解析失败则忽略游标，从最新开始查询
			zap.L().Warn("parse cursor failed, ignore cursor", zap.String("cursor", cursor), zap.Error(err))
		} else {
			cursorTime := time.Unix(timestamp, 0)
			query = query.Where("created_at < ?", cursorTime)
		}
	}

	// 按时间倒序（最新的在前）
	// 多查一条用于判断是否有更多
	if err := query.
		Order("created_at DESC").
		Limit(pageSize + 1).
		Find(&messages).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "游标分页查询群消息 receive_id=%s", receiveId)
	}

	// 判断是否有更多
	hasMore := len(messages) > pageSize
	if hasMore {
		messages = messages[:pageSize] // 截取实际需要的数据
	}

	// 生成下一页游标
	var nextCursor string
	if len(messages) > 0 && hasMore {
		nextCursor = strconv.FormatInt(messages[len(messages)-1].CreatedAt.Unix(), 10)
	}

	return &model.CursorPageMessageResult{
		Messages:   messages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
