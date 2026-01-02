package repository

import (
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/enum/contact_apply/contact_apply_status_enum"

	"gorm.io/gorm"
)

type contactApplyRepository struct {
	db *gorm.DB
}

// NewContactApplyRepository 创建联系人申请 Repository
func NewContactApplyRepository(db *gorm.DB) ContactApplyRepository {
	return &contactApplyRepository{db: db}
}

// FindByUuid 按 UUID 查找申请
func (r *contactApplyRepository) FindByUuid(uuid string) (*model.ContactApply, error) {
	var apply model.ContactApply
	if err := r.db.First(&apply, "uuid = ?", uuid).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询申请 uuid=%s", uuid)
	}
	return &apply, nil
}

// FindByApplicantIdAndTargetId 按申请人和目标查找
func (r *contactApplyRepository) FindByApplicantIdAndTargetId(applicantId, targetId string) (*model.ContactApply, error) {
	var apply model.ContactApply
	if err := r.db.Where("applicant_id = ? AND target_id = ?", applicantId, targetId).First(&apply).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return &apply, nil
}

// FindByTargetIdPending 查找待处理的申请
func (r *contactApplyRepository) FindByTargetIdPending(targetId string) ([]model.ContactApply, error) {
	var applies []model.ContactApply
	if err := r.db.Where("target_id = ? AND status = ?", targetId, contact_apply_status_enum.PENDING).Find(&applies).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询待处理申请 target_id=%s", targetId)
	}
	return applies, nil
}

// FindByTargetIdAndType 按目标和类型查找
func (r *contactApplyRepository) FindByTargetIdAndType(targetId string, contactType int8) ([]model.ContactApply, error) {
	var applies []model.ContactApply
	if err := r.db.Where("target_id = ? AND contact_type = ? AND status = ?", targetId, contactType, contact_apply_status_enum.PENDING).Find(&applies).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询申请 target_id=%s type=%d", targetId, contactType)
	}
	return applies, nil
}

// Create 创建申请
func (r *contactApplyRepository) Create(apply *model.ContactApply) error {
	if err := r.db.Create(apply).Error; err != nil {
		return wrapDBError(err, "创建联系人申请")
	}
	return nil
}

// Update 更新申请
func (r *contactApplyRepository) Update(apply *model.ContactApply) error {
	if err := r.db.Save(apply).Error; err != nil {
		return wrapDBError(err, "更新联系人申请")
	}
	return nil
}

// UpdateStatus 更新申请状态
func (r *contactApplyRepository) UpdateStatus(uuid string, status int8) error {
	if err := r.db.Model(&model.ContactApply{}).Where("uuid = ?", uuid).Update("status", status).Error; err != nil {
		return wrapDBErrorf(err, "更新申请状态 uuid=%s", uuid)
	}
	return nil
}

// SoftDelete 软删除申请
func (r *contactApplyRepository) SoftDelete(applicantId, targetId string) error {
	if err := r.db.Where("applicant_id = ? AND target_id = ?", applicantId, targetId).Delete(&model.ContactApply{}).Error; err != nil {
		return wrapDBErrorf(err, "删除申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return nil
}

// SoftDeleteByUsers 批量按用户IDs软删除联系人申请
func (r *contactApplyRepository) SoftDeleteByUsers(userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := r.db.Where("applicant_id IN ? OR target_id IN ?", userUuids, userUuids).Delete(&model.ContactApply{}).Error; err != nil {
		return wrapDBError(err, "批量删除联系人申请")
	}
	return nil
}
