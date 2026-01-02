package repository

import (
	"kama_chat_server/internal/model"

	"gorm.io/gorm"
)

type sessionRepository struct {
	db *gorm.DB
}

// NewSessionRepository 创建会话 Repository
func NewSessionRepository(db *gorm.DB) SessionRepository {
	return &sessionRepository{db: db}
}

// FindByUuid 按 UUID 查找会话
func (r *sessionRepository) FindByUuid(uuid string) (*model.Session, error) {
	var session model.Session
	if err := r.db.First(&session, "uuid = ?", uuid).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询会话 uuid=%s", uuid)
	}
	return &session, nil
}

// FindBySendIdAndReceiveId 按发送者和接收者查找会话
func (r *sessionRepository) FindBySendIdAndReceiveId(sendId, receiveId string) (*model.Session, error) {
	var session model.Session
	if err := r.db.Where("send_id = ? AND receive_id = ?", sendId, receiveId).First(&session).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询会话 send_id=%s receive_id=%s", sendId, receiveId)
	}
	return &session, nil
}

// FindBySendId 按发送者查找所有会话
func (r *sessionRepository) FindBySendId(sendId string) ([]model.Session, error) {
	var sessions []model.Session
	if err := r.db.Where("send_id = ?", sendId).Find(&sessions).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询会话列表 send_id=%s", sendId)
	}
	return sessions, nil
}

// FindByReceiveId 按接收者查找所有会话
func (r *sessionRepository) FindByReceiveId(receiveId string) ([]model.Session, error) {
	var sessions []model.Session
	if err := r.db.Where("receive_id = ?", receiveId).Find(&sessions).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询会话列表 receive_id=%s", receiveId)
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

// Update 更新会话
func (r *sessionRepository) Update(session *model.Session) error {
	if err := r.db.Save(session).Error; err != nil {
		return wrapDBError(err, "更新会话")
	}
	return nil
}

// SoftDeleteByUuids 批量软删除会话
func (r *sessionRepository) SoftDeleteByUuids(uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Where("uuid IN ?", uuids).Delete(&model.Session{}).Error; err != nil {
		return wrapDBError(err, "批量删除会话")
	}
	return nil
}

// SoftDeleteByUsers 批量按用户IDs软删除会话
func (r *sessionRepository) SoftDeleteByUsers(userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := r.db.Where("send_id IN ? OR receive_id IN ?", userUuids, userUuids).Delete(&model.Session{}).Error; err != nil {
		return wrapDBError(err, "批量删除会话")
	}
	return nil
}

// UpdateByReceiveId 批量更新会话（按接收者ID）
func (r *sessionRepository) UpdateByReceiveId(receiveId string, updates map[string]interface{}) error {
	if err := r.db.Model(&model.Session{}).Where("receive_id = ?", receiveId).Updates(updates).Error; err != nil {
		return wrapDBErrorf(err, "批量更新会话 receive_id=%s", receiveId)
	}
	return nil
}
