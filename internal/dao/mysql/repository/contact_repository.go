// Package repository 提供数据访问层的具体实现
// 本文件实现 ContactRepository 接口，处理联系人关系相关的数据库操作
package repository

import (
	"kama_chat_server/internal/model"

	"gorm.io/gorm"
)

// contactRepository ContactRepository 接口的实现
type contactRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewContactRepository 创建 ContactRepository 实例
func NewContactRepository(db *gorm.DB) ContactRepository {
	return &contactRepository{db: db}
}

// FindByUserIdAndContactId 根据用户ID和联系人ID查找关系
// 用于检查两人是否为好友
func (r *contactRepository) FindByUserIdAndContactId(userId, contactId string) (*model.Contact, error) {
	var contact model.Contact
	if err := r.db.Where("user_id = ? AND contact_id = ?", userId, contactId).First(&contact).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询联系人 user_id=%s contact_id=%s", userId, contactId)
	}
	return &contact, nil
}

// FindByUserIdAndType 根据用户ID和联系人类型查找
// contactType: 0=好友, 1=群组
func (r *contactRepository) FindByUserIdAndType(userId string, contactType int8) ([]model.Contact, error) {
	var contacts []model.Contact
	if err := r.db.Where("user_id = ? AND contact_type = ?", userId, contactType).Find(&contacts).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询联系人列表 user_id=%s type=%d", userId, contactType)
	}
	return contacts, nil
}

// FindUsersByContactId 根据联系人ID反向查找
// 用于查找某个用户/群组被哪些人添加为好友
func (r *contactRepository) FindUsersByContactId(contactId string) ([]model.Contact, error) {
	var contacts []model.Contact
	if err := r.db.Where("contact_id = ?", contactId).Find(&contacts).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询联系人 contact_id=%s", contactId)
	}
	return contacts, nil
}

// CreateContact 创建联系人关系
func (r *contactRepository) CreateContact(contact *model.Contact) error {
	if err := r.db.Create(contact).Error; err != nil {
		return wrapDBError(err, "创建联系人关系")
	}
	return nil
}

// UpdateStatus 更新联系人状态
// status: 见 model.Contact 中的状态定义
func (r *contactRepository) UpdateStatus(userId, contactId string, status int8) error {
	if err := r.db.Model(&model.Contact{}).Where("user_id = ? AND contact_id = ?", userId, contactId).Update("status", status).Error; err != nil {
		return wrapDBErrorf(err, "更新联系人状态 user_id=%s contact_id=%s", userId, contactId)
	}
	return nil
}

// SoftDelete 软删除联系人关系
func (r *contactRepository) SoftDelete(userId, contactId string) error {
	if err := r.db.Where("user_id = ? AND contact_id = ?", userId, contactId).Delete(&model.Contact{}).Error; err != nil {
		return wrapDBErrorf(err, "删除联系人关系 user_id=%s contact_id=%s", userId, contactId)
	}
	return nil
}

// SoftDeleteByUsers 批量软删除指定用户的所有联系人关系
// 删除该用户添加的和被该用户添加的所有关系
func (r *contactRepository) SoftDeleteByUsers(userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	// 使用 OR 条件删除双向关系
	if err := r.db.Where("user_id IN ? OR contact_id IN ?", userUuids, userUuids).Delete(&model.Contact{}).Error; err != nil {
		return wrapDBError(err, "批量删除联系人关系")
	}
	return nil
}
