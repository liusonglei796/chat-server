// Package message 提供消息相关数据访问层的具体实现
// 本文件实现 MessageRepository 接口，处理消息相关的数据库操作
package message

import (
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/errorx"

	"gorm.io/gorm"
)

// messageRepository MessageRepository 接口的实现
type messageRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewMessageRepository 创建 MessageRepository 实例
// db: GORM 数据库实例
// 返回: MessageRepository 接口实现
func NewMessageRepository(db *gorm.DB) *messageRepository {
	return &messageRepository{db: db}
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
		return nil, errorx.WrapDBErrorf(err, "查询消息 user1=%s user2=%s", userOneId, userTwoId)
	}
	return messages, nil
}

// FindByUserIdsPaged 根据两个用户ID查找私聊消息（分页）
// userOneId, userTwoId: 两个用户的 UUID
// page: 页码（从1开始）
// pageSize: 每页数量
// 返回: 消息列表和错误
func (r *messageRepository) FindByUserIdsPaged(userOneId, userTwoId string, page, pageSize int) ([]model.Message, error) {
	var messages []model.Message
	// 计算偏移量
	offset := (page - 1) * pageSize
	// 使用 OR 条件查找双向消息，按时间倒序排列（最新的在前）
	if err := r.db.Where("(send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)",
		userOneId, userTwoId, userTwoId, userOneId).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&messages).Error; err != nil {
		return nil, errorx.WrapDBErrorf(err, "查询消息 user1=%s user2=%s", userOneId, userTwoId)
	}
	return messages, nil
}

// FindByGroupIdPaged 根据群组ID分页查找群聊消息
// receiveId: 群组 UUID
// page: 页码（从1开始）
// pageSize: 每页数量
// 返回: 消息列表、总数和错误
func (r *messageRepository) FindByGroupIdPaged(receiveId string, page, pageSize int) ([]model.Message, int64, error) {
	var messages []model.Message
	var total int64

	// 统计总数
	if err := r.db.Model(&model.Message{}).Where("receive_id = ?", receiveId).Count(&total).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "统计群消息数量 receive_id=%s", receiveId)
	}

	// 计算偏移量并分页查询，按时间倒序（最新的在前）
	offset := (page - 1) * pageSize
	if err := r.db.Where("receive_id = ?", receiveId).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&messages).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "分页查询群消息 receive_id=%s", receiveId)
	}
	return messages, total, nil
}

// UpdateStatus 更新消息状态
// uuid: 消息唯一标识
// status: 新状态值
// 返回: 操作错误
func (r *messageRepository) UpdateStatus(uuid string, status int8) error {
	if err := r.db.Model(&model.Message{}).Where("uuid = ?", uuid).Update("status", status).Error; err != nil {
		return errorx.WrapDBErrorf(err, "更新消息状态 uuid=%s", uuid)
	}
	return nil
}

// Create 创建新消息
// message: 消息结构体
// 返回: 操作错误
func (r *messageRepository) Create(message *model.Message) error {
	if err := r.db.Create(message).Error; err != nil {
		return errorx.WrapDBError(err, "创建消息")
	}
	return nil
}

// FindLastMessageByUserIds 获取两人之间最后一条消息
func (r *messageRepository) FindLastMessageByUserIds(userOneId, userTwoId string) (*model.Message, error) {
	var message model.Message
	if err := r.db.Where("(send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)",
		userOneId, userTwoId, userTwoId, userOneId).
		Order("created_at DESC").First(&message).Error; err != nil {
		if errorx.IsNotFound(err) {
			return nil, nil // 没有消息，返回 nil 而不是错误
		}
		return nil, errorx.WrapDBErrorf(err, "查询私聊最后消息 failed")
	}
	return &message, nil
}

// FindLastMessageByGroupId 获取群组最后一条消息
func (r *messageRepository) FindLastMessageByGroupId(groupId string) (*model.Message, error) {
	var message model.Message
	if err := r.db.Where("receive_id = ?", groupId).
		Order("created_at DESC").First(&message).Error; err != nil {
		if errorx.IsNotFound(err) {
			return nil, nil // 没有消息
		}
		return nil, errorx.WrapDBErrorf(err, "查询群聊最后消息 failed")
	}
	return &message, nil
}
