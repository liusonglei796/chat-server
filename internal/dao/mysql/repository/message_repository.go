package repository

import (
	"kama_chat_server/internal/model"

	"gorm.io/gorm"
)

type messageRepository struct {
	db *gorm.DB
}

// NewMessageRepository 创建消息 Repository
func NewMessageRepository(db *gorm.DB) MessageRepository {
	return &messageRepository{db: db}
}

// FindBySessionId 按会话ID查找消息
func (r *messageRepository) FindBySessionId(sessionId string) ([]model.Message, error) {
	var messages []model.Message
	if err := r.db.Where("session_id = ?", sessionId).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询消息 session_id=%s", sessionId)
	}
	return messages, nil
}

// FindByUserIds 按发送者和接收者查找消息（双向）
func (r *messageRepository) FindByUserIds(userOneId, userTwoId string) ([]model.Message, error) {
	var messages []model.Message
	if err := r.db.Where("(send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)",
		userOneId, userTwoId, userTwoId, userOneId).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询消息 user1=%s user2=%s", userOneId, userTwoId)
	}
	return messages, nil
}

// FindByGroupId 按接收者ID查找消息（群聊）
func (r *messageRepository) FindByGroupId(receiveId string) ([]model.Message, error) {
	var messages []model.Message
	if err := r.db.Where("receive_id = ?", receiveId).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询群消息 receive_id=%s", receiveId)
	}
	return messages, nil
}

// Create 创建消息
func (r *messageRepository) Create(message *model.Message) error {
	if err := r.db.Create(message).Error; err != nil {
		return wrapDBError(err, "创建消息")
	}
	return nil
}
