package user

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/internal/infra/outbox"
)

// Store 用户数据访问接口
type Store interface {
	FindByUuid(ctx context.Context, uuid string) (*UserInfo, error)
	FindByTelephone(ctx context.Context, telephone string) (*UserInfo, error)
	FindByNickname(ctx context.Context, nickname string) (*UserInfo, error)
	FindByUuids(ctx context.Context, uuids []string) ([]UserInfo, error)
	CreateUser(ctx context.Context, user *UserInfo) error
	UpdateUserInfo(ctx context.Context, user *UserInfo) error
	UpdateUserStatusByUuids(ctx context.Context, uuids []string, status int8) error
	UpdateUserIsAdminByUuids(ctx context.Context, uuids []string, isAdmin int8) error
	SoftDeleteUserByUuids(ctx context.Context, uuids []string) error
	FindAllPaged(ctx context.Context, page, pageSize int, keyword string, status *int8) ([]UserInfo, int64, error)
}

type storeImpl struct {
	db *gorm.DB
}

// NewStore 创建 Store 实例
func NewStore(db *gorm.DB) Store {
	return &storeImpl{db: db}
}

func (r *storeImpl) FindByUuid(ctx context.Context, uuid string) (*UserInfo, error) {
	var user UserInfo
	if err := r.db.WithContext(ctx).First(&user, "uuid = ?", uuid).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询用户 uuid=%s", uuid)
	}
	return &user, nil
}

func (r *storeImpl) FindByTelephone(ctx context.Context, telephone string) (*UserInfo, error) {
	var user UserInfo
	if err := r.db.WithContext(ctx).First(&user, "telephone = ?", telephone).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询用户 telephone=%s", telephone)
	}
	return &user, nil
}

func (r *storeImpl) FindByNickname(ctx context.Context, nickname string) (*UserInfo, error) {
	var user UserInfo
	if err := r.db.WithContext(ctx).First(&user, "nickname = ?", nickname).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询用户 nickname=%s", nickname)
	}
	return &user, nil
}

func (r *storeImpl) FindByUuids(ctx context.Context, uuids []string) ([]UserInfo, error) {
	var users []UserInfo
	if err := r.db.WithContext(ctx).Where("uuid IN ?", uuids).Find(&users).Error; err != nil {
		return nil, dberr.WrapDBError(err, "批量查询用户")
	}
	return users, nil
}

func (r *storeImpl) CreateUser(ctx context.Context, user *UserInfo) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		return dberr.WrapDBError(err, "创建用户")
	}
	return nil
}

func (r *storeImpl) UpdateUserInfo(ctx context.Context, user *UserInfo) error {
	if err := r.db.WithContext(ctx).Save(user).Error; err != nil {
		return dberr.WrapDBError(err, "更新用户信息")
	}
	return nil
}

func (r *storeImpl) UpdateUserStatusByUuids(ctx context.Context, uuids []string, status int8) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&UserInfo{}).Where("uuid IN ?", uuids).Update("status", status).Error; err != nil {
		return dberr.WrapDBError(err, "批量更新用户状态")
	}
	return nil
}

func (r *storeImpl) UpdateUserIsAdminByUuids(ctx context.Context, uuids []string, isAdmin int8) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&UserInfo{}).Where("uuid IN ?", uuids).Update("is_admin", isAdmin).Error; err != nil {
		return dberr.WrapDBError(err, "批量更新用户管理员状态")
	}
	return nil
}

func (r *storeImpl) SoftDeleteUserByUuids(ctx context.Context, uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Where("uuid IN ?", uuids).Delete(&UserInfo{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除用户")
	}
	return nil
}

func (r *storeImpl) FindAllPaged(ctx context.Context, page, pageSize int, keyword string, status *int8) ([]UserInfo, int64, error) {
	var users []UserInfo
	var total int64

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).Model(&UserInfo{}).Unscoped()
	if keyword != "" {
		query = query.Where("nickname LIKE ? OR telephone LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBError(err, "统计用户数量")
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&users).Error; err != nil {
		return nil, 0, dberr.WrapDBError(err, "分页查询用户")
	}

	return users, total, nil
}

// ==================== 工作单元 Unit of Work ====================

// UoW 用户领域事务工作单元
type UoW interface {
	WithTx(fn func(tx any) error) error
	RecordEvent(ctx context.Context, aggregateId, eventType string, payload []byte) error
	UserStore() Store
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

func (u *uowImpl) UserStore() Store {
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
