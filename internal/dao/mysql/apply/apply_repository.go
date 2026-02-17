// Package apply 提供申请相关数据访问层的具体实现
// 本文件实现 ApplyRepository 接口，处理联系人申请相关的数据库操作
package apply

import (
	"kama_chat_server/internal/dao/mysql/dberr"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/enum/apply/apply_status"

	"gorm.io/gorm"
)

// applyRepository ApplyRepository 接口的实现
type applyRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewApplyRepository 创建 ApplyRepository 实例
func NewApplyRepository(db *gorm.DB) *applyRepository {
	return &applyRepository{db: db}
}

// FindByApplicantIdAndTargetId 根据申请人和目标查找申请
// 用于检查是否已存在申请记录
// applicantId: 申请人 UUID
// targetId: 目标 UUID（用户或群组）
func (r *applyRepository) FindByApplicantIdAndTargetId(applicantId, targetId string) (*model.Apply, error) {
	var apply model.Apply
	if err := r.db.Where("applicant_id = ? AND target_id = ?", applicantId, targetId).First(&apply).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return &apply, nil
}

// FindByTargetIdPendingPaged 分页查找目标用户的待处理申请
// targetId: 目标用户/群组 UUID
// page: 页码（从1开始）
// pageSize: 每页数量
// 返回: 申请列表、总数、错误
func (r *applyRepository) FindByTargetIdPendingPaged(targetId string, page, pageSize int) ([]model.Apply, int64, error) {
	var applies []model.Apply
	var total int64

	// 校验分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 构建查询条件
	query := r.db.Model(&model.Apply{}).Where("target_id = ? AND status = ?", targetId, apply_status.PENDING)

	// 先统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计待处理申请数量 target_id=%s", targetId)
	}

	// 计算偏移量并分页查询，按申请时间倒序（最新的在前）
	offset := (page - 1) * pageSize
	if err := r.db.Where("target_id = ? AND status = ?", targetId, apply_status.PENDING).
		Order("last_apply_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&applies).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询待处理申请 target_id=%s", targetId)
	}

	return applies, total, nil
}

// CreateApply 创建新的申请记录
func (r *applyRepository) CreateApply(apply *model.Apply) error {
	if err := r.db.Create(apply).Error; err != nil {
		return dberr.WrapDBError(err, "创建联系人申请")
	}
	return nil
}

// Update 更新申请记录（全字段更新）
func (r *applyRepository) Update(apply *model.Apply) error {
	if err := r.db.Save(apply).Error; err != nil {
		return dberr.WrapDBError(err, "更新联系人申请")
	}
	return nil
}

// SoftDelete 软删除申请记录
func (r *applyRepository) SoftDelete(applicantId, targetId string) error {
	if err := r.db.Where("applicant_id = ? AND target_id = ?", applicantId, targetId).Delete(&model.Apply{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "删除申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return nil
}

// SoftDeleteByUsers 批量软删除指定用户的所有申请
// 删除用户发出的和收到的所有申请
func (r *applyRepository) SoftDeleteByUsers(userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	// 使用 OR 条件删除用户发出和收到的所有申请
	if err := r.db.Where("applicant_id IN ? OR target_id IN ?", userUuids, userUuids).Delete(&model.Apply{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除联系人申请")
	}
	return nil
}
