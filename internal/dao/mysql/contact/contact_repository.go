// Package repository 提供数据访问层的具体实现
// 本文件实现 ContactRepository 接口，处理联系人关系相关的数据库操作
package contact

import (
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/errorx"

	"gorm.io/gorm"
)

// contactRepository ContactRepository 接口的实现
type contactRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewContactRepository 创建 ContactRepository 实例
func NewContactRepository(db *gorm.DB) *contactRepository {
	return &contactRepository{db: db}
}

// FindByUserIdAndContactId 根据用户ID和联系人ID查找关系
// 用于检查两人是否为好友或群组成员关系
func (r *contactRepository) FindByUserIdAndContactId(userId, contactId string, contactType int8) (*model.Contact, error) {
	var contact model.Contact
	if err := r.db.Where("user_id = ? AND contact_id = ? AND contact_type = ?", userId, contactId, contactType).First(&contact).Error; err != nil {
		return nil, errorx.WrapDBErrorf(err, "查询联系人 user_id=%s contact_id=%s type=%d", userId, contactId, contactType)
	}
	return &contact, nil
}

// IsFriend 判断两个用户是否互为好友
// 检查两条记录都存在：user1->user2 和 user2->user1，且类型为好友（contact_type=0），状态都为正常
func (r *contactRepository) IsFriend(userId1, userId2 string) (bool, error) {
	var count int64
	// 检查双向关系：A添加B 且 B添加A，且类型为好友，状态都为正常
	if err := r.db.Model(&model.Contact{}).
		Where("(user_id = ? AND contact_id = ? AND contact_type = 0) OR (user_id = ? AND contact_id = ? AND contact_type = 0)",
			userId1, userId2, userId2, userId1).
		Where("status = ?", 0).
		Count(&count).Error; err != nil {
		return false, errorx.WrapDBError(err, "check friend relationship failed")
	}
	return count == 2, nil
}

// FindByUserIdAndType 根据用户ID和联系人类型查找
// contactType: 0=好友, 1=群组
func (r *contactRepository) FindByUserIdAndType(userId string, contactType int8, page, pageSize int) ([]model.Contact, int64, error) {
	var contacts []model.Contact
	var total int64

	query := r.db.Model(&model.Contact{}).
		Where("user_id = ? AND contact_type = ?", userId, contactType)

	// 查询总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "查询联系人总数 user_id=%s type=%d", userId, contactType)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&contacts).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "查询联系人列表 user_id=%s type=%d", userId, contactType)
	}

	return contacts, total, nil
}

// FindUsersByContactId 根据联系人ID反向查找
// 用于查找某个用户/群组被哪些人添加为好友
func (r *contactRepository) FindUsersByContactId(contactId string) ([]model.Contact, error) {
	var contacts []model.Contact
	if err := r.db.Where("contact_id = ?", contactId).Find(&contacts).Error; err != nil {
		return nil, errorx.WrapDBErrorf(err, "查询联系人 contact_id=%s", contactId)
	}
	return contacts, nil
}

// CreateContact 创建联系人关系
func (r *contactRepository) CreateContact(contact *model.Contact) error {
	if err := r.db.Create(contact).Error; err != nil {
		return errorx.WrapDBError(err, "创建联系人关系")
	}
	return nil
}

// UpdateStatus 更新联系人状态
// status: 见 model.Contact 中的状态定义
func (r *contactRepository) UpdateStatus(userId, contactId string, contactType int8, status int8) error {
	if err := r.db.Model(&model.Contact{}).
		Where("user_id = ? AND contact_id = ? AND contact_type = ?", userId, contactId, contactType).
		Update("status", status).Error; err != nil {
		return errorx.WrapDBErrorf(err, "更新联系人状态 user_id=%s contact_id=%s type=%d", userId, contactId, contactType)
	}
	return nil
}

// SoftDelete 软删除联系人关系
func (r *contactRepository) SoftDelete(userId, contactId string, contactType int8) error {
	if err := r.db.Where("user_id = ? AND contact_id = ? AND contact_type = ?", userId, contactId, contactType).
		Delete(&model.Contact{}).Error; err != nil {
		return errorx.WrapDBErrorf(err, "删除联系人关系 user_id=%s contact_id=%s type=%d", userId, contactId, contactType)
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
		return errorx.WrapDBError(err, "批量删除联系人关系")
	}
	return nil
}
