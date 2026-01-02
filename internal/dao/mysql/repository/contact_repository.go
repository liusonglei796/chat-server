package repository

import (
	"kama_chat_server/internal/model"

	"gorm.io/gorm"
)

type contactRepository struct {
	db *gorm.DB
}

// NewContactRepository 创建联系人 Repository
func NewContactRepository(db *gorm.DB) ContactRepository {
	return &contactRepository{db: db}
}

// FindByUserIdAndContactId 按用户ID和联系人ID查找
func (r *contactRepository) FindByUserIdAndContactId(userId, contactId string) (*model.UserContact, error) {
	var contact model.UserContact
	if err := r.db.Where("user_id = ? AND contact_id = ?", userId, contactId).First(&contact).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询联系人 user_id=%s contact_id=%s", userId, contactId)
	}
	return &contact, nil
}

// FindByUserId 按用户ID查找所有联系人
func (r *contactRepository) FindByUserId(userId string) ([]model.UserContact, error) {
	var contacts []model.UserContact
	if err := r.db.Where("user_id = ?", userId).Find(&contacts).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询联系人列表 user_id=%s", userId)
	}
	return contacts, nil
}

// FindByUserIdAndType 按用户ID和联系人类型查找
func (r *contactRepository) FindByUserIdAndType(userId string, contactType int8) ([]model.UserContact, error) {
	var contacts []model.UserContact
	if err := r.db.Where("user_id = ? AND contact_type = ?", userId, contactType).Find(&contacts).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询联系人列表 user_id=%s type=%d", userId, contactType)
	}
	return contacts, nil
}

// FindByContactId 按联系人ID查找
func (r *contactRepository) FindByContactId(contactId string) ([]model.UserContact, error) {
	var contacts []model.UserContact
	if err := r.db.Where("contact_id = ?", contactId).Find(&contacts).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询联系人 contact_id=%s", contactId)
	}
	return contacts, nil
}

// Create 创建联系人关系
func (r *contactRepository) Create(contact *model.UserContact) error {
	if err := r.db.Create(contact).Error; err != nil {
		return wrapDBError(err, "创建联系人关系")
	}
	return nil
}

// Update 更新联系人关系
func (r *contactRepository) Update(contact *model.UserContact) error {
	if err := r.db.Save(contact).Error; err != nil {
		return wrapDBError(err, "更新联系人关系")
	}
	return nil
}

// UpdateStatus 更新联系人状态
func (r *contactRepository) UpdateStatus(userId, contactId string, status int8) error {
	if err := r.db.Model(&model.UserContact{}).Where("user_id = ? AND contact_id = ?", userId, contactId).Update("status", status).Error; err != nil {
		return wrapDBErrorf(err, "更新联系人状态 user_id=%s contact_id=%s", userId, contactId)
	}
	return nil
}

// SoftDelete 软删除联系人关系
func (r *contactRepository) SoftDelete(userId, contactId string) error {
	if err := r.db.Where("user_id = ? AND contact_id = ?", userId, contactId).Delete(&model.UserContact{}).Error; err != nil {
		return wrapDBErrorf(err, "删除联系人关系 user_id=%s contact_id=%s", userId, contactId)
	}
	return nil
}

// SoftDeleteByUsers 批量按用户IDs软删除联系人关系
func (r *contactRepository) SoftDeleteByUsers(userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := r.db.Where("user_id IN ? OR contact_id IN ?", userUuids, userUuids).Delete(&model.UserContact{}).Error; err != nil {
		return wrapDBError(err, "批量删除联系人关系")
	}
	return nil
}
