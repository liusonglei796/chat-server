// Package group 提供群组相关数据访问层的具体实现
// 本文件实现 GroupRepository 接口，处理群组相关的数据库操作
package group

import (
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/errorx"

	"gorm.io/gorm"
)

// groupRepository GroupRepository 接口的实现
type groupRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewGroupRepository 创建 GroupRepository 实例
func NewGroupRepository(db *gorm.DB) *groupRepository {
	return &groupRepository{db: db}
}

// FindByUuid 根据 UUID 查找群组
func (r *groupRepository) FindByUuid(uuid string) (*model.GroupInfo, error) {
	var group model.GroupInfo
	if err := r.db.First(&group, "uuid = ?", uuid).Error; err != nil {
		return nil, errorx.WrapDBErrorf(err, "查询群组 uuid=%s", uuid)
	}
	return &group, nil
}

// FindByOwnerIdPaged 根据群主ID分页查找其创建的群组
// ownerId: 群主用户UUID
// page: 页码（从1开始）
// pageSize: 每页数量
// 返回: 群组列表、总数、错误
func (r *groupRepository) FindByOwnerIdPaged(ownerId string, page, pageSize int) ([]model.GroupInfo, int64, error) {
	var groups []model.GroupInfo
	var total int64

	// 先统计总数
	if err := r.db.Model(&model.GroupInfo{}).Where("owner_id = ?", ownerId).Count(&total).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "统计群组数量 owner_id=%s", ownerId)
	}

	// 计算偏移量并分页查询
	offset := (page - 1) * pageSize
	if err := r.db.Where("owner_id = ?", ownerId).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&groups).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "分页查询群组 owner_id=%s", ownerId)
	}

	return groups, total, nil
}

// GetGroupList [管理员] 分页查找群组（包含软删除的）
// page: 页码（从1开始）
// pageSize: 每页数量
// 返回: 群组列表、总数、错误
func (r *groupRepository) GetGroupList(page, pageSize int) ([]model.GroupInfo, int64, error) {
	var groups []model.GroupInfo
	var total int64
	// 计算偏移量
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	// 先查询总数
	if err := r.db.Unscoped().Model(&model.GroupInfo{}).Count(&total).Error; err != nil {
		return nil, 0, errorx.WrapDBError(err, "查询群组总数")
	}

	// 再分页查询
	if err := r.db.Unscoped().Model(&model.GroupInfo{}).Offset(offset).Limit(pageSize).Find(&groups).Error; err != nil {
		return nil, 0, errorx.WrapDBError(err, "分页查询群组")
	}

	return groups, total, nil
}

// FindByUuids 根据UUID列表批量查找群组
func (r *groupRepository) FindByUuids(uuids []string) ([]model.GroupInfo, error) {
	var groups []model.GroupInfo
	if err := r.db.Where("uuid IN ?", uuids).Find(&groups).Error; err != nil {
		return nil, errorx.WrapDBError(err, "批量查询群组")
	}
	return groups, nil
}

// CreateGroup 创建群组
func (r *groupRepository) CreateGroup(group *model.GroupInfo) error {
	if err := r.db.Create(group).Error; err != nil {
		return errorx.WrapDBError(err, "创建群组")
	}
	return nil
}

// Update 更新群组信息（全字段更新）
func (r *groupRepository) Update(group *model.GroupInfo) error {
	if err := r.db.Save(group).Error; err != nil {
		return errorx.WrapDBError(err, "更新群组")
	}
	return nil
}

// UpdateStatusByUuids [管理员] 批量更新群组状态
// status: 0=正常, 1=禁用, 2=解散
func (r *groupRepository) UpdateStatusByUuids(uuids []string, status int8) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Model(&model.GroupInfo{}).Where("uuid IN ?", uuids).Update("status", status).Error; err != nil {
		return errorx.WrapDBError(err, "批量更新群组状态")
	}
	return nil
}

// IncrementMemberCount 增加群成员计数
// 使用 UpdateColumn + gorm.Expr 实现原子自增
func (r *groupRepository) IncrementMemberCount(uuid string) error {
	if err := r.db.Model(&model.GroupInfo{}).Where("uuid = ?", uuid).UpdateColumn("member_cnt", gorm.Expr("member_cnt + ?", 1)).Error; err != nil {
		return errorx.WrapDBErrorf(err, "增加群成员数 uuid=%s", uuid)
	}
	return nil
}

// DecrementMemberCountBy 减少指定数量的群成员计数
// 用于批量踢人时更新计数
func (r *groupRepository) DecrementMemberCountBy(uuid string, count int) error {
	if count <= 0 {
		return nil
	}
	if err := r.db.Model(&model.GroupInfo{}).Where("uuid = ?", uuid).UpdateColumn("member_cnt", gorm.Expr("member_cnt - ?", count)).Error; err != nil {
		return errorx.WrapDBErrorf(err, "减少群成员数 uuid=%s count=%d", uuid, count)
	}
	return nil
}

// SoftDeleteByUuids [管理员] 批量软删除群组
func (r *groupRepository) SoftDeleteByUuids(uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Where("uuid IN ?", uuids).Delete(&model.GroupInfo{}).Error; err != nil {
		return errorx.WrapDBError(err, "批量删除群组")
	}
	return nil
}
