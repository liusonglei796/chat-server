// Package apply 提供申请相关数据访问层的具体实现
// 本文件实现 ApplyStore 接口，处理联系人申请相关的数据库操作
package apply

import (
	"context"
	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/enum/apply/apply_status"

	"gorm.io/gorm"
)

// applyStore ApplyStore 接口的实现
type applyStore struct {
	db *gorm.DB // GORM 数据库实例
}

// NewApplyStore 创建 ApplyStore 实例
func NewApplyStore(db *gorm.DB) *applyStore {
	return &applyStore{db: db}
}

// FindByApplicantIdAndTargetId 根据申请人和目标查找申请
// 用于检查是否已存在申请记录
// applicantId: 申请人 UUID
// targetId: 目标 UUID（用户或群组）
func (r *applyStore) FindByApplicantIdAndTargetId(ctx context.Context, applicantId, targetId string) (*model.Apply, error) {
	var apply model.Apply
	if err := r.db.WithContext(ctx).Where("applicant_id = ? AND target_id = ?", applicantId, targetId).First(&apply).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return &apply, nil
}

// FindByTargetIdPendingPaged 分页查找目标用户的待处理申请
// targetId: 目标用户/群组 UUID
// page: 页码（从1开始）
// pageSize: 每页数量
// 返回: 申请列表、总数、错误
func (r *applyStore) FindByTargetIdPendingPaged(ctx context.Context, targetId string, page, pageSize int) ([]model.Apply, int64, error) {
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
	query := r.db.WithContext(ctx).Model(&model.Apply{}).Where("target_id = ? AND status = ?", targetId, apply_status.PENDING)

	// 先统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计待处理申请数量 target_id=%s", targetId)
	}

	// 计算偏移量并分页查询，按申请时间倒序（最新的在前）
	offset := (page - 1) * pageSize
	if err := r.db.WithContext(ctx).Where("target_id = ? AND status = ?", targetId, apply_status.PENDING).
		Order("last_apply_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&applies).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询待处理申请 target_id=%s", targetId)
	}

	return applies, total, nil
}

// CreateApply 创建新的申请记录
func (r *applyStore) CreateApply(ctx context.Context, apply *model.Apply) error {
	if err := r.db.WithContext(ctx).Create(apply).Error; err != nil {
		return dberr.WrapDBError(err, "创建联系人申请")
	}
	return nil
}

// Update 更新申请记录（全字段更新）
func (r *applyStore) Update(ctx context.Context, apply *model.Apply) error {
	if err := r.db.WithContext(ctx).Save(apply).Error; err != nil {
		return dberr.WrapDBError(err, "更新联系人申请")
	}
	return nil
}

// UpdateStatusIfPending 仅在申请状态为 PENDING 且版本匹配时原子更新为目标状态（状态机与版本号双重乐观锁）
// 来源: PassFriendApply / PassGroupApply / Refuse / Black 审批跃迁
// 用于: 并发防重、ABA 阻断与迟到审批穿透拦截，仅当状态为 PENDING 且版本号匹配时更新，依赖 RowsAffected > 0 判定成功
func (r *applyStore) UpdateStatusIfPending(ctx context.Context, uuid string, toStatus int8, expectedVersion int32) (bool, error) {
	// Step 1.1: 构建更新载荷与递增版本表达式
	// 来源: 目标状态 toStatus
	// 用于: 原子跃迁申请状态并递增版本号
	updates := map[string]interface{}{
		"status":  toStatus,
		"version": gorm.Expr("version + 1"),
	}

	// Step 1.2: 执行带 uuid、pending 状态及版本号约束的原子更新
	// 来源: uuid, apply_status.PENDING, expectedVersion
	// 用于: 防重试穿透与生命周期跨代串号，产出影响行数校验结果
	query := r.db.WithContext(ctx).Model(&model.Apply{}).
		Where("uuid = ? AND status = ?", uuid, apply_status.PENDING)
	if expectedVersion > 0 {
		query = query.Where("version = ?", expectedVersion)
	}

	result := query.Updates(updates)
	if result.Error != nil {
		return false, dberr.WrapDBErrorf(result.Error, "状态机更新申请状态 uuid=%s version=%d", uuid, expectedVersion)
	}
	return result.RowsAffected > 0, nil
}

// SoftDelete 删除申请记录（物理删除，释放唯一索引槽位避免再次申请冲突）
func (r *applyStore) SoftDelete(ctx context.Context, applicantId, targetId string) error {
	if err := r.db.WithContext(ctx).Unscoped().Where("applicant_id = ? AND target_id = ?", applicantId, targetId).Delete(&model.Apply{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "删除申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return nil
}

// SoftDeleteByUsers 批量删除指定用户的所有申请（物理删除，释放唯一索引槽位）
// 删除用户发出的和收到的所有申请
func (r *applyStore) SoftDeleteByUsers(ctx context.Context, userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	// 使用 OR 条件删除用户发出和收到的所有申请
	if err := r.db.WithContext(ctx).Unscoped().Where("applicant_id IN ? OR target_id IN ?", userUuids, userUuids).Delete(&model.Apply{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除联系人申请")
	}
	return nil
}
