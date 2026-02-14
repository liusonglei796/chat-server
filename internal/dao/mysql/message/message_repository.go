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

// FindByUserIdsPaged 根据两个用户ID查找私聊消息（分页）
// userOneId, userTwoId: 两个用户的 UUID
// page: 页码（从1开始）
// pageSize: 每页数量
// 返回: 消息列表、总数和错误
func (r *messageRepository) FindByUserIdsPaged(userOneId, userTwoId string, page, pageSize int) ([]model.Message, int64, error) {
	var messages []model.Message
	var total int64

	condition := "(send_id = ? AND receive_id = ?) OR (send_id = ? AND receive_id = ?)"

	// 统计总数
	if err := r.db.Model(&model.Message{}).Where(condition,
		userOneId, userTwoId, userTwoId, userOneId).Count(&total).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "统计私聊消息数量 user1=%s user2=%s", userOneId, userTwoId)
	}

	// 计算偏移量
	offset := (page - 1) * pageSize
	// 使用 OR 条件查找双向消息，按时间倒序排列（最新的在前）
	if err := r.db.Where(condition,
		userOneId, userTwoId, userTwoId, userOneId).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&messages).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "查询消息 user1=%s user2=%s", userOneId, userTwoId)
	}
	return messages, total, nil
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

// FindByUuid 根据消息UUID查找消息
// uuid: 消息唯一标识
// 返回: 消息实体和错误
func (r *messageRepository) FindByUuid(uuid string) (*model.Message, error) {
	var msg model.Message
	if err := r.db.Where("uuid = ?", uuid).First(&msg).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errorx.New(errorx.CodeNotFound, "消息不存在")
		}
		return nil, errorx.WrapDBErrorf(err, "查找消息 uuid=%s", uuid)
	}
	return &msg, nil
}

// UpdateContent 更新消息内容和类型（用于撤回）
// uuid: 消息唯一标识
// content: 新内容
// msgType: 新消息类型
// 返回: 操作错误
func (r *messageRepository) UpdateContent(uuid, content string, msgType int8) error {
	if err := r.db.Model(&model.Message{}).Where("uuid = ?", uuid).
		Updates(map[string]interface{}{
			"content": content,
			"type":    msgType,
		}).Error; err != nil {
		return errorx.WrapDBErrorf(err, "更新消息内容 uuid=%s", uuid)
	}
	return nil
}
