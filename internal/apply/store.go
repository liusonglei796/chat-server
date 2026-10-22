package apply

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/internal/infra/outbox"
	"kama_chat_server/pkg/enum/apply/apply_status"
)

// Store 申请数据访问接口
type Store interface {
	FindByApplicantIdAndTargetId(ctx context.Context, applicantId, targetId string) (*Apply, error)
	FindByTargetIdPendingPaged(ctx context.Context, targetId string, page, pageSize int) ([]Apply, int64, error)
	CreateApply(ctx context.Context, apply *Apply) error
	Update(ctx context.Context, apply *Apply) error
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
	if err := r.db.WithContext(ctx).Where("applicant_id = ? AND target_id = ?", applicantId, targetId).First(&a).Error; err != nil {
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

	query := r.db.WithContext(ctx).Model(&Apply{}).Where("target_id = ? AND status = ?", targetId, apply_status.PENDING)
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
	if err := r.db.WithContext(ctx).Create(apply).Error; err != nil {
		return dberr.WrapDBErrorf(err, "创建申请 applicant_id=%s target_id=%s", apply.ApplicantId, apply.TargetId)
	}
	return nil
}

func (r *storeImpl) Update(ctx context.Context, apply *Apply) error {
	if err := r.db.WithContext(ctx).Save(apply).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新申请 uuid=%s", apply.Uuid)
	}
	return nil
}

func (r *storeImpl) UpdateStatusIfPending(ctx context.Context, uuid string, toStatus int8, expectedVersion int32) (bool, error) {
	res := r.db.WithContext(ctx).Model(&Apply{}).
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
	if err := r.db.WithContext(ctx).Where("applicant_id = ? AND target_id = ?", applicantId, targetId).Delete(&Apply{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "软删除申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return nil
}

func (r *storeImpl) SoftDeleteByUsers(ctx context.Context, userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Where("applicant_id IN ? OR target_id IN ?", userUuids, userUuids).Delete(&Apply{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "批量软删除申请 user_count=%d", len(userUuids))
	}
	return nil
}

// ==================== 工作单元 Unit of Work ====================

// UoW 申请领域事务工作单元
type UoW interface {
	WithTx(fn func(tx any) error) error
	RecordEvent(ctx context.Context, aggregateId, eventType string, payload []byte) error
	ApplyStore() Store
}

type uowImpl struct {
	db    *gorm.DB
	store Store
}

// NewUoW 创建工作单元实例
func NewUoW(db *gorm.DB) UoW {
	return &uowImpl{
		db:    db,
		store: NewStore(db),
	}
}

func (u *uowImpl) ApplyStore() Store {
	return u.store
}

func (u *uowImpl) RecordEvent(ctx context.Context, aggregateId, eventType string, payload []byte) error {
	return outbox.RecordEvent(ctx, u.db, aggregateId, eventType, payload)
}

func (u *uowImpl) WithTx(fn func(tx any) error) error {
	return u.db.Transaction(func(tx *gorm.DB) error {
		return fn(NewUoW(tx))
	})
}

// WithTx 泛型事务辅助函数
func WithTx[T any](exec UoW, fn func(tx T) error) error {
	return exec.WithTx(func(tx any) error {
		t, ok := tx.(T)
		if !ok {
			return fmt.Errorf("transaction callback type mismatch: got %T, want %T", tx, *new(T))
		}
		return fn(t)
	})
}
