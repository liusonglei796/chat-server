// Package repository 提供数据访问层的具体实现
// 本文件实现 MessageRepository 接口，处理消息相关的数据库操作
package repository

import (
	"kama_chat_server/internal/model"

	"gorm.io/gorm"
)

// messageRepository MessageRepository 接口的实现
type messageRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewMessageRepository 创建 MessageRepository 实例
// db: GORM 数据库实例
// 返回: MessageRepository 接口实现
func NewMessageRepository(db *gorm.DB) MessageRepository {
	return &messageRepository{db: db}
}

// FindBySessionId 根据会话ID查找消息
// 按创建时间升序排列（最旧的在前）
// sessionId: 会话 UUID
// 返回: 消息列表和错误
func (r *messageRepository) FindBySessionId(sessionId string) ([]model.Message, error) {
	var messages []model.Message
	// Order("created_at ASC"): 按时间升序，便于按顺序显示聊天记录
	if err := r.db.Where("session_id = ?", sessionId).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询消息 session_id=%s", sessionId)
	}
	return messages, nil
}

// FindByUserIds 根据两个用户ID查找私聊消息（双向）
// 查找 A->B 和 B->A 的所有消息
// userOneId, userTwoId: 两个用户的 UUID
// 返回: 消息列表和错误
func (r *messageRepository) FindByUserIds(userOneId, userTwoId string) ([]model.Message, error) {
	var messages []model.Message
	// 使用 OR 条件查找双向消息
	if err := r.db.Where("(send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)",
		userOneId, userTwoId, userTwoId, userOneId).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询消息 user1=%s user2=%s", userOneId, userTwoId)
	}
	return messages, nil
}

// FindByGroupId 根据群组ID查找群聊消息
// 群聊消息的 receive_id 为群组 UUID
// receiveId: 群组 UUID
// 返回: 消息列表和错误
func (r *messageRepository) FindByGroupId(receiveId string) ([]model.Message, error) {
	var messages []model.Message
	if err := r.db.Where("receive_id = ?", receiveId).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询群消息 receive_id=%s", receiveId)
	}
	return messages, nil
}

// Create 创建新消息
// message: 消息模型
// 返回: 操作错误
func (r *messageRepository) Create(message *model.Message) error {
	if err := r.db.Create(message).Error; err != nil {
		return wrapDBError(err, "创建消息")
	}
	return nil
}
