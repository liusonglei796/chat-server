// Package repository 提供数据访问层的具体实现
// 本文件实现 SessionRepository 接口，处理会话相关的数据库操作
package repository

import (
	"kama_chat_server/internal/model"

	"gorm.io/gorm"
)

// sessionRepository SessionRepository 接口的实现
type sessionRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewSessionRepository 创建 SessionRepository 实例
func NewSessionRepository(db *gorm.DB) SessionRepository {
	return &sessionRepository{db: db}
}

// FindBySendIdAndReceiveId 根据发送者和接收者查找会话
// 用于查找两个实体之间是否已存在会话
func (r *sessionRepository) FindBySendIdAndReceiveId(sendId, receiveId string) (*model.Session, error) {
	var session model.Session
	if err := r.db.Where("send_id = ? AND receive_id = ?", sendId, receiveId).First(&session).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询会话 send_id=%s receive_id=%s", sendId, receiveId)
	}
	return &session, nil
}

// FindBySendId 根据发送者查找所有会话
// 用于获取用户的会话列表
func (r *sessionRepository) FindBySendId(sendId string) ([]model.Session, error) {
	var sessions []model.Session
	if err := r.db.Where("send_id = ?", sendId).Find(&sessions).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询会话列表 send_id=%s", sendId)
	}
	return sessions, nil
}

// Create 创建会话
func (r *sessionRepository) Create(session *model.Session) error {
	if err := r.db.Create(session).Error; err != nil {
		return wrapDBError(err, "创建会话")
	}
	return nil
}

// SoftDeleteByUuids 批量软删除会话（按照会话ID）
func (r *sessionRepository) SoftDeleteByUuids(uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Where("uuid IN ?", uuids).Delete(&model.Session{}).Error; err != nil {
		return wrapDBError(err, "批量删除会话")
	}
	return nil
}

// SoftDeleteByUsers 批量软删除指定用户的所有会话
// 删除用户发起的和接收的所有会话
func (r *sessionRepository) SoftDeleteByUsers(userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := r.db.Where("send_id IN ? OR receive_id IN ?", userUuids, userUuids).Delete(&model.Session{}).Error; err != nil {
		return wrapDBError(err, "批量删除会话")
	}
	return nil
}

// UpdateByReceiveId 根据接收者ID批量更新会话字段
// 用于群组信息变更时同步更新相关会话
// updates: 要更新的字段 map，如 {"receive_name": "新群名", "avatar": "新头像"}
func (r *sessionRepository) UpdateByReceiveId(receiveId string, updates map[string]interface{}) error {
	if err := r.db.Model(&model.Session{}).Where("receive_id = ?", receiveId).Updates(updates).Error; err != nil {
		return wrapDBErrorf(err, "批量更新会话 receive_id=%s", receiveId)
	}
	return nil
}
