// Package repository 提供数据访问层的具体实现
// 本文件实现 ApplyRepository 接口，处理联系人申请相关的数据库操作
package repository

import (
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/enum/contact_apply/contact_apply_status_enum"

	"gorm.io/gorm"
)

// applyRepository ApplyRepository 接口的实现
type applyRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewApplyRepository 创建 ApplyRepository 实例
func NewApplyRepository(db *gorm.DB) ApplyRepository {
	return &applyRepository{db: db}
}

// FindByApplicantIdAndTargetId 根据申请人和目标查找申请
// 用于检查是否已存在申请记录
// applicantId: 申请人 UUID
// targetId: 目标 UUID（用户或群组）
func (r *applyRepository) FindByApplicantIdAndTargetId(applicantId, targetId string) (*model.Apply, error) {
	var apply model.Apply
	if err := r.db.Where("applicant_id = ? AND target_id = ?", applicantId, targetId).First(&apply).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return &apply, nil
}

// FindByTargetIdPending 查找目标用户的待处理申请
// 用于获取收到的好友/入群请求列表
// targetId: 目标用户/群组 UUID
func (r *applyRepository) FindByTargetIdPending(targetId string) ([]model.Apply, error) {
	var applies []model.Apply
	// 只查询状态为 PENDING 的申请
	if err := r.db.Where("target_id = ? AND status = ?", targetId, contact_apply_status_enum.PENDING).Find(&applies).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询待处理申请 target_id=%s", targetId)
	}
	return applies, nil
}

// Create 创建新的申请记录
func (r *applyRepository) Create(apply *model.Apply) error {
	if err := r.db.Create(apply).Error; err != nil {
		return wrapDBError(err, "创建联系人申请")
	}
	return nil
}

// Update 更新申请记录（全字段更新）
func (r *applyRepository) Update(apply *model.Apply) error {
	if err := r.db.Save(apply).Error; err != nil {
		return wrapDBError(err, "更新联系人申请")
	}
	return nil
}

// SoftDelete 软删除申请记录
func (r *applyRepository) SoftDelete(applicantId, targetId string) error {
	if err := r.db.Where("applicant_id = ? AND target_id = ?", applicantId, targetId).Delete(&model.Apply{}).Error; err != nil {
		return wrapDBErrorf(err, "删除申请 applicant_id=%s target_id=%s", applicantId, targetId)
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
		return wrapDBError(err, "批量删除联系人申请")
	}
	return nil
}
