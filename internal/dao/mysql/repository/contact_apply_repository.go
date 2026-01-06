// Package repository 提供数据访问层的具体实现
// 本文件实现 ContactApplyRepository 接口，处理联系人申请相关的数据库操作
package repository

import (
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/enum/contact_apply/contact_apply_status_enum"

	"gorm.io/gorm"
)

// contactApplyRepository ContactApplyRepository 接口的实现
type contactApplyRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewContactApplyRepository 创建 ContactApplyRepository 实例
func NewContactApplyRepository(db *gorm.DB) ContactApplyRepository {
	return &contactApplyRepository{db: db}
}

// FindByUuid 根据 UUID 查找申请记录
func (r *contactApplyRepository) FindByUuid(uuid string) (*model.ContactApply, error) {
	var apply model.ContactApply
	if err := r.db.First(&apply, "uuid = ?", uuid).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询申请 uuid=%s", uuid)
	}
	return &apply, nil
}

// FindByApplicantIdAndTargetId 根据申请人和目标查找申请
// 用于检查是否已存在申请记录
// applicantId: 申请人 UUID
// targetId: 目标 UUID（用户或群组）
func (r *contactApplyRepository) FindByApplicantIdAndTargetId(applicantId, targetId string) (*model.ContactApply, error) {
	var apply model.ContactApply
	if err := r.db.Where("applicant_id = ? AND target_id = ?", applicantId, targetId).First(&apply).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return &apply, nil
}

// FindByTargetIdPending 查找目标用户的待处理申请
// 用于获取收到的好友/入群请求列表
// targetId: 目标用户/群组 UUID
func (r *contactApplyRepository) FindByTargetIdPending(targetId string) ([]model.ContactApply, error) {
	var applies []model.ContactApply
	// 只查询状态为 PENDING 的申请
	if err := r.db.Where("target_id = ? AND status = ?", targetId, contact_apply_status_enum.PENDING).Find(&applies).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询待处理申请 target_id=%s", targetId)
	}
	return applies, nil
}

// FindByTargetIdAndType 根据目标和申请类型查找待处理申请
// contactType: 0=好友申请, 1=入群申请
func (r *contactApplyRepository) FindByTargetIdAndType(targetId string, contactType int8) ([]model.ContactApply, error) {
	var applies []model.ContactApply
	if err := r.db.Where("target_id = ? AND contact_type = ? AND status = ?", targetId, contactType, contact_apply_status_enum.PENDING).Find(&applies).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询申请 target_id=%s type=%d", targetId, contactType)
	}
	return applies, nil
}

// Create 创建新的申请记录
func (r *contactApplyRepository) Create(apply *model.ContactApply) error {
	if err := r.db.Create(apply).Error; err != nil {
		return wrapDBError(err, "创建联系人申请")
	}
	return nil
}

// Update 更新申请记录（全字段更新）
func (r *contactApplyRepository) Update(apply *model.ContactApply) error {
	if err := r.db.Save(apply).Error; err != nil {
		return wrapDBError(err, "更新联系人申请")
	}
	return nil
}

// UpdateStatus 更新申请状态
// status: 0=待处理, 1=已通过, 2=已拒绝, 3=已拉黑
func (r *contactApplyRepository) UpdateStatus(uuid string, status int8) error {
	if err := r.db.Model(&model.ContactApply{}).Where("uuid = ?", uuid).Update("status", status).Error; err != nil {
		return wrapDBErrorf(err, "更新申请状态 uuid=%s", uuid)
	}
	return nil
}

// SoftDelete 软删除申请记录
func (r *contactApplyRepository) SoftDelete(applicantId, targetId string) error {
	if err := r.db.Where("applicant_id = ? AND target_id = ?", applicantId, targetId).Delete(&model.ContactApply{}).Error; err != nil {
		return wrapDBErrorf(err, "删除申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return nil
}

// SoftDeleteByUsers 批量软删除指定用户的所有申请
// 删除用户发出的和收到的所有申请
func (r *contactApplyRepository) SoftDeleteByUsers(userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	// 使用 OR 条件删除用户发出和收到的所有申请
	if err := r.db.Where("applicant_id IN ? OR target_id IN ?", userUuids, userUuids).Delete(&model.ContactApply{}).Error; err != nil {
		return wrapDBError(err, "批量删除联系人申请")
	}
	return nil
}
