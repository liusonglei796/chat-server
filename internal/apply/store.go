package apply

import (
	"context"

	"gorm.io/gorm"

	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/pkg/enum/apply/apply_status"
)

// Store 申请数据访问接口
type Store interface {
	FindByApplicantIdAndTargetId(ctx context.Context, applicantId, targetId string) (*Apply, error)
	FindByTargetIdPendingPaged(ctx context.Context, targetId string, page, pageSize int) ([]Apply, int64, error)
	CreateApply(ctx context.Context, apply *Apply) error
	Update(ctx context.Context, apply *Apply) error
	UpdateWithVersion(ctx context.Context, apply *Apply, expectedVersion int32) (bool, error)
	UpdateStatusIfPending(ctx context.Context, uuid string, toStatus int8, expectedVersion int32) (bool, error)
	SoftDelete(ctx context.Context, applicantId, targetId string) error
	SoftDeleteByUsers(ctx context.Context, userUuids []string) error
}

type storeImpl struct {
	db *gorm.DB
}

// NewStore 创建 Store 实例
func NewStore(db *gorm.DB) Store {
	return &storeImpl{db: db}
}

func (r *storeImpl) FindByApplicantIdAndTargetId(ctx context.Context, applicantId, targetId string) (*Apply, error) {
	var a Apply
	if err := database.GetDB(ctx, r.db).Where("applicant_id = ? AND target_id = ?", applicantId, targetId).First(&a).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return &a, nil
}

func (r *storeImpl) FindByTargetIdPendingPaged(ctx context.Context, targetId string, page, pageSize int) ([]Apply, int64, error) {
	var applies []Apply
	var total int64

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := database.GetDB(ctx, r.db).Model(&Apply{}).Where("target_id = ? AND status = ?", targetId, apply_status.PENDING)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计待处理申请数量 target_id=%s", targetId)
	}

	offset := (page - 1) * pageSize
	if err := query.Order("last_apply_at DESC").Offset(offset).Limit(pageSize).Find(&applies).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询待处理申请 target_id=%s", targetId)
	}
	return applies, total, nil
}

func (r *storeImpl) CreateApply(ctx context.Context, apply *Apply) error {
	if err := database.GetDB(ctx, r.db).Create(apply).Error; err != nil {
		return dberr.WrapDBErrorf(err, "创建申请 applicant_id=%s target_id=%s", apply.ApplicantId, apply.TargetId)
	}
	return nil
}

func (r *storeImpl) Update(ctx context.Context, apply *Apply) error {
	if err := database.GetDB(ctx, r.db).Save(apply).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新申请 uuid=%s", apply.Uuid)
	}
	return nil
}

func (r *storeImpl) UpdateWithVersion(ctx context.Context, apply *Apply, expectedVersion int32) (bool, error) {
	res := database.GetDB(ctx, r.db).Model(&Apply{}).
		Where("id = ? AND version = ?", apply.ID, expectedVersion).
		Updates(map[string]interface{}{
			"uuid":          apply.Uuid,
			"last_apply_at": apply.LastApplyAt,
			"status":        apply.Status,
			"message":       apply.Message,
			"version":       expectedVersion + 1,
		})
	if res.Error != nil {
		return false, dberr.WrapDBErrorf(res.Error, "CAS 更新申请记录 id=%d", apply.ID)
	}
	return res.RowsAffected > 0, nil
}

func (r *storeImpl) UpdateStatusIfPending(ctx context.Context, uuid string, toStatus int8, expectedVersion int32) (bool, error) {
	res := database.GetDB(ctx, r.db).Model(&Apply{}).
		Where("uuid = ? AND status = ? AND version = ?", uuid, apply_status.PENDING, expectedVersion).
		Updates(map[string]interface{}{
			"status":  toStatus,
			"version": expectedVersion + 1,
		})
	if res.Error != nil {
		return false, dberr.WrapDBErrorf(res.Error, "CAS 更新申请状态 uuid=%s", uuid)
	}
	return res.RowsAffected > 0, nil
}

func (r *storeImpl) SoftDelete(ctx context.Context, applicantId, targetId string) error {
	if err := database.GetDB(ctx, r.db).Where("applicant_id = ? AND target_id = ?", applicantId, targetId).Delete(&Apply{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "软删除申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return nil
}

func (r *storeImpl) SoftDeleteByUsers(ctx context.Context, userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := database.GetDB(ctx, r.db).Where("applicant_id IN ? OR target_id IN ?", userUuids, userUuids).Delete(&Apply{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "批量软删除申请 user_count=%d", len(userUuids))
	}
	return nil
}
