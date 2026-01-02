package repository

import (
	"kama_chat_server/internal/model"

	"gorm.io/gorm"
)

type groupRepository struct {
	db *gorm.DB
}

// NewGroupRepository 创建群组 Repository
func NewGroupRepository(db *gorm.DB) GroupRepository {
	return &groupRepository{db: db}
}

// FindByUuid 按 UUID 查找群组
func (r *groupRepository) FindByUuid(uuid string) (*model.GroupInfo, error) {
	var group model.GroupInfo
	if err := r.db.First(&group, "uuid = ?", uuid).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询群组 uuid=%s", uuid)
	}
	return &group, nil
}

// FindByOwnerId 按群主 ID 查找群组
func (r *groupRepository) FindByOwnerId(ownerId string) ([]model.GroupInfo, error) {
	var groups []model.GroupInfo
	if err := r.db.Where("owner_id = ?", ownerId).Find(&groups).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询群组 owner_id=%s", ownerId)
	}
	return groups, nil
}

// FindAll 查找所有群组
func (r *groupRepository) FindAll() ([]model.GroupInfo, error) {
	var groups []model.GroupInfo
	if err := r.db.Unscoped().Find(&groups).Error; err != nil {
		return nil, wrapDBError(err, "查询所有群组")
	}
	return groups, nil
}

// GetList 分页查找群组
func (r *groupRepository) GetList(page, pageSize int) ([]model.GroupInfo, int64, error) {
	var groups []model.GroupInfo
	var total int64

	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	db := r.db.Unscoped().Model(&model.GroupInfo{})

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, wrapDBError(err, "查询群组总数")
	}

	if err := db.Offset(offset).Limit(pageSize).Find(&groups).Error; err != nil {
		return nil, 0, wrapDBError(err, "分页查询群组")
	}

	return groups, total, nil
}

// FindByUuids 按 UUID 列表查找群组
func (r *groupRepository) FindByUuids(uuids []string) ([]model.GroupInfo, error) {
	var groups []model.GroupInfo
	if err := r.db.Where("uuid IN ?", uuids).Find(&groups).Error; err != nil {
		return nil, wrapDBError(err, "批量查询群组")
	}
	return groups, nil
}

// Create 创建群组
func (r *groupRepository) Create(group *model.GroupInfo) error {
	if err := r.db.Create(group).Error; err != nil {
		return wrapDBError(err, "创建群组")
	}
	return nil
}

// Update 更新群组
func (r *groupRepository) Update(group *model.GroupInfo) error {
	if err := r.db.Save(group).Error; err != nil {
		return wrapDBError(err, "更新群组")
	}
	return nil
}

// UpdateStatus 更新群组状态
func (r *groupRepository) UpdateStatus(uuid string, status int8) error {
	if err := r.db.Model(&model.GroupInfo{}).Where("uuid = ?", uuid).Update("status", status).Error; err != nil {
		return wrapDBErrorf(err, "更新群组状态 uuid=%s", uuid)
	}
	return nil
}

// UpdateStatusByUuids 批量更新群组状态
func (r *groupRepository) UpdateStatusByUuids(uuids []string, status int8) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Model(&model.GroupInfo{}).Where("uuid IN ?", uuids).Update("status", status).Error; err != nil {
		return wrapDBError(err, "批量更新群组状态")
	}
	return nil
}

// IncrementMemberCount 增加群成员数
func (r *groupRepository) IncrementMemberCount(uuid string) error {
	if err := r.db.Model(&model.GroupInfo{}).Where("uuid = ?", uuid).UpdateColumn("member_cnt", gorm.Expr("member_cnt + ?", 1)).Error; err != nil {
		return wrapDBErrorf(err, "增加群成员数 uuid=%s", uuid)
	}
	return nil
}

// DecrementMemberCount 减少群成员数
func (r *groupRepository) DecrementMemberCount(uuid string) error {
	if err := r.db.Model(&model.GroupInfo{}).Where("uuid = ?", uuid).UpdateColumn("member_cnt", gorm.Expr("member_cnt - ?", 1)).Error; err != nil {
		return wrapDBErrorf(err, "减少群成员数 uuid=%s", uuid)
	}
	return nil
}

// DecrementMemberCountBy 减少群成员数（指定数量）
func (r *groupRepository) DecrementMemberCountBy(uuid string, count int) error {
	if count <= 0 {
		return nil
	}
	if err := r.db.Model(&model.GroupInfo{}).Where("uuid = ?", uuid).UpdateColumn("member_cnt", gorm.Expr("member_cnt - ?", count)).Error; err != nil {
		return wrapDBErrorf(err, "减少群成员数 uuid=%s count=%d", uuid, count)
	}
	return nil
}

// SoftDeleteByUuids 批量软删除群组
func (r *groupRepository) SoftDeleteByUuids(uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Where("uuid IN ?", uuids).Delete(&model.GroupInfo{}).Error; err != nil {
		return wrapDBError(err, "批量删除群组")
	}
	return nil
}
