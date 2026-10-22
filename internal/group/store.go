package group

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/internal/infra/outbox"
	"kama_chat_server/pkg/errorx"
)

// GroupStore 群组信息数据访问接口
type GroupStore interface {
	FindByUuid(ctx context.Context, uuid string) (*GroupInfo, error)
	FindByOwnerIdPaged(ctx context.Context, ownerId string, page, pageSize int) ([]GroupInfo, int64, error)
	FindByOwnerIdAndName(ctx context.Context, ownerId, name string) (*GroupInfo, error)
	GetGroupList(ctx context.Context, page, pageSize int) ([]GroupInfo, int64, error)
	FindByUuids(ctx context.Context, uuids []string) ([]GroupInfo, error)
	CreateGroup(ctx context.Context, group *GroupInfo) error
	Update(ctx context.Context, group *GroupInfo) error
	UpdateWithVersion(ctx context.Context, uuid string, expectedVersion int32, updates map[string]interface{}) error
	UpdateStatusByUuids(ctx context.Context, uuids []string, status int8) error
	IncrementMemberCount(ctx context.Context, uuid string) error
	DecrementMemberCountBy(ctx context.Context, uuid string, count int) error
	SoftDeleteByUuids(ctx context.Context, uuids []string) error
}

type groupStoreImpl struct {
	db *gorm.DB
}

// NewGroupStore 创建 GroupStore 实例
func NewGroupStore(db *gorm.DB) GroupStore {
	return &groupStoreImpl{db: db}
}

func (r *groupStoreImpl) FindByUuid(ctx context.Context, uuid string) (*GroupInfo, error) {
	var g GroupInfo
	if err := r.db.WithContext(ctx).First(&g, "uuid = ?", uuid).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询群组 uuid=%s", uuid)
	}
	return &g, nil
}

func (r *groupStoreImpl) FindByOwnerIdPaged(ctx context.Context, ownerId string, page, pageSize int) ([]GroupInfo, int64, error) {
	var groups []GroupInfo
	var total int64

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	if err := r.db.WithContext(ctx).Model(&GroupInfo{}).Where("owner_id = ?", ownerId).Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计群组数量 owner_id=%s", ownerId)
	}

	offset := (page - 1) * pageSize
	if err := r.db.WithContext(ctx).Where("owner_id = ?", ownerId).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&groups).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询群组 owner_id=%s", ownerId)
	}

	return groups, total, nil
}

func (r *groupStoreImpl) FindByOwnerIdAndName(ctx context.Context, ownerId, name string) (*GroupInfo, error) {
	var g GroupInfo
	if err := r.db.WithContext(ctx).Where("owner_id = ? AND name = ? AND status = 0", ownerId, name).First(&g).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询群组 owner_id=%s name=%s", ownerId, name)
	}
	return &g, nil
}

func (r *groupStoreImpl) GetGroupList(ctx context.Context, page, pageSize int) ([]GroupInfo, int64, error) {
	var groups []GroupInfo
	var total int64

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	if err := r.db.WithContext(ctx).Unscoped().Model(&GroupInfo{}).Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBError(err, "查询群组总数")
	}

	if err := r.db.WithContext(ctx).Unscoped().Model(&GroupInfo{}).Offset(offset).Limit(pageSize).Find(&groups).Error; err != nil {
		return nil, 0, dberr.WrapDBError(err, "分页查询群组")
	}

	return groups, total, nil
}

func (r *groupStoreImpl) FindByUuids(ctx context.Context, uuids []string) ([]GroupInfo, error) {
	var groups []GroupInfo
	if err := r.db.WithContext(ctx).Where("uuid IN ?", uuids).Find(&groups).Error; err != nil {
		return nil, dberr.WrapDBError(err, "批量查询群组")
	}
	return groups, nil
}

func (r *groupStoreImpl) CreateGroup(ctx context.Context, group *GroupInfo) error {
	if err := r.db.WithContext(ctx).Create(group).Error; err != nil {
		return dberr.WrapDBError(err, "创建群组")
	}
	return nil
}

func (r *groupStoreImpl) Update(ctx context.Context, group *GroupInfo) error {
	if err := r.db.WithContext(ctx).Save(group).Error; err != nil {
		return dberr.WrapDBError(err, "更新群组")
	}
	return nil
}

func (r *groupStoreImpl) UpdateWithVersion(ctx context.Context, uuid string, expectedVersion int32, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	updates["version"] = gorm.Expr("version + ?", 1)
	result := r.db.WithContext(ctx).Model(&GroupInfo{}).
		Where("uuid = ? AND version = ?", uuid, expectedVersion).
		Updates(updates)
	if result.Error != nil {
		return dberr.WrapDBErrorf(result.Error, "乐观锁更新群组 uuid=%s version=%d", uuid, expectedVersion)
	}
	if result.RowsAffected == 0 {
		return errorx.New(errorx.CodeConflict, "群资料已被他人修改，请刷新后重试")
	}
	return nil
}

func (r *groupStoreImpl) UpdateStatusByUuids(ctx context.Context, uuids []string, status int8) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&GroupInfo{}).Where("uuid IN ?", uuids).Update("status", status).Error; err != nil {
		return dberr.WrapDBError(err, "批量更新群组状态")
	}
	return nil
}

func (r *groupStoreImpl) IncrementMemberCount(ctx context.Context, uuid string) error {
	if err := r.db.WithContext(ctx).Model(&GroupInfo{}).Where("uuid = ?", uuid).UpdateColumn("member_cnt", gorm.Expr("member_cnt + ?", 1)).Error; err != nil {
		return dberr.WrapDBErrorf(err, "增加群成员数 uuid=%s", uuid)
	}
	return nil
}

func (r *groupStoreImpl) DecrementMemberCountBy(ctx context.Context, uuid string, count int) error {
	if count <= 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&GroupInfo{}).Where("uuid = ?", uuid).UpdateColumn("member_cnt", gorm.Expr("GREATEST(0, member_cnt - ?)", count)).Error; err != nil {
		return dberr.WrapDBErrorf(err, "减少群成员数 uuid=%s count=%d", uuid, count)
	}
	return nil
}

func (r *groupStoreImpl) SoftDeleteByUuids(ctx context.Context, uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Unscoped().Where("uuid IN ?", uuids).Delete(&GroupInfo{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除群组")
	}
	return nil
}

// ==================== GroupMemberStore ====================

// GroupMemberStore 群成员数据访问接口
type GroupMemberStore interface {
	FindByGroupUuid(ctx context.Context, groupUuid string) ([]GroupMember, error)
	FindMembersWithUserInfoPaged(ctx context.Context, groupUuid string, page, pageSize int) ([]GroupMemberWithUserInfo, int64, error)
	FindByGroupAndUser(ctx context.Context, groupUuid, userUuid string) (*GroupMember, error)
	CreateGroupMember(ctx context.Context, member *GroupMember) error
	DeleteByGroupUuid(ctx context.Context, groupUuid string) error
	DeleteByUserUuids(ctx context.Context, groupUuid string, userUuids []string) (int64, error)
	DeleteByGroupUuids(ctx context.Context, groupUuids []string) error
	GetMemberIdsByGroupUuids(ctx context.Context, groupUuids []string) ([]string, error)
	FindGroupUuidsByUserPaged(ctx context.Context, userUuid string, page, pageSize int) ([]string, int64, error)
	UpdateMuteUntil(ctx context.Context, groupUuid, userUuid string, muteUntil *time.Time) error
}

type groupMemberStoreImpl struct {
	db *gorm.DB
}

// NewGroupMemberStore 创建 GroupMemberStore 实例
func NewGroupMemberStore(db *gorm.DB) GroupMemberStore {
	return &groupMemberStoreImpl{db: db}
}

func (r *groupMemberStoreImpl) FindByGroupUuid(ctx context.Context, groupUuid string) ([]GroupMember, error) {
	var members []GroupMember
	if err := r.db.WithContext(ctx).Where("group_uuid = ?", groupUuid).Find(&members).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询群成员 group_uuid=%s", groupUuid)
	}
	return members, nil
}

func (r *groupMemberStoreImpl) FindMembersWithUserInfoPaged(ctx context.Context, groupUuid string, page, pageSize int) ([]GroupMemberWithUserInfo, int64, error) {
	var members []GroupMemberWithUserInfo
	var total int64

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	if err := r.db.WithContext(ctx).Table("group_member").
		Where("group_uuid = ? AND deleted_at IS NULL", groupUuid).
		Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计群成员数量 group_uuid=%s", groupUuid)
	}

	offset := (page - 1) * pageSize
	if err := r.db.WithContext(ctx).Table("group_member").
		Select("user_info.uuid as user_id, user_info.nickname, user_info.avatar").
		Joins("LEFT JOIN user_info ON group_member.user_uuid = user_info.uuid").
		Where("group_member.group_uuid = ? AND group_member.deleted_at IS NULL", groupUuid).
		Offset(offset).
		Limit(pageSize).
		Scan(&members).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询群成员详情 group_uuid=%s", groupUuid)
	}
	return members, total, nil
}

func (r *groupMemberStoreImpl) FindByGroupAndUser(ctx context.Context, groupUuid, userUuid string) (*GroupMember, error) {
	var member GroupMember
	if err := r.db.WithContext(ctx).Where("group_uuid = ? AND user_uuid = ?", groupUuid, userUuid).First(&member).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询群成员 group_uuid=%s user_uuid=%s", groupUuid, userUuid)
	}
	return &member, nil
}

func (r *groupMemberStoreImpl) CreateGroupMember(ctx context.Context, member *GroupMember) error {
	if err := r.db.WithContext(ctx).Create(member).Error; err != nil {
		return dberr.WrapDBError(err, "创建群成员")
	}
	return nil
}

func (r *groupMemberStoreImpl) DeleteByGroupUuid(ctx context.Context, groupUuid string) error {
	if err := r.db.WithContext(ctx).Unscoped().Where("group_uuid = ?", groupUuid).Delete(&GroupMember{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "删除群所有成员 group_uuid=%s", groupUuid)
	}
	return nil
}

func (r *groupMemberStoreImpl) DeleteByUserUuids(ctx context.Context, groupUuid string, userUuids []string) (int64, error) {
	if len(userUuids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Unscoped().Where("group_uuid = ? AND user_uuid IN ?", groupUuid, userUuids).Delete(&GroupMember{})
	if err := res.Error; err != nil {
		return 0, dberr.WrapDBErrorf(err, "批量删除群成员 group_uuid=%s", groupUuid)
	}
	return res.RowsAffected, nil
}

func (r *groupMemberStoreImpl) DeleteByGroupUuids(ctx context.Context, groupUuids []string) error {
	if len(groupUuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Unscoped().Where("group_uuid IN ?", groupUuids).Delete(&GroupMember{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除群所有成员")
	}
	return nil
}

func (r *groupMemberStoreImpl) GetMemberIdsByGroupUuids(ctx context.Context, groupUuids []string) ([]string, error) {
	var members []string
	if len(groupUuids) == 0 {
		return members, nil
	}
	if err := r.db.WithContext(ctx).Model(&GroupMember{}).Distinct("user_uuid").Where("group_uuid IN ?", groupUuids).Pluck("user_uuid", &members).Error; err != nil {
		return nil, dberr.WrapDBError(err, "批量查询群成员ID")
	}
	return members, nil
}

func (r *groupMemberStoreImpl) FindGroupUuidsByUserPaged(ctx context.Context, userUuid string, page, pageSize int) ([]string, int64, error) {
	var total int64

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	if err := r.db.WithContext(ctx).Model(&GroupMember{}).
		Where("user_uuid = ?", userUuid).
		Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计用户群组数量 user_uuid=%s", userUuid)
	}

	offset := (page - 1) * pageSize
	var groupUuids []string
	if err := r.db.WithContext(ctx).Model(&GroupMember{}).
		Where("user_uuid = ?", userUuid).
		Offset(offset).
		Limit(pageSize).
		Pluck("group_uuid", &groupUuids).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询用户群组 user_uuid=%s", userUuid)
	}
	return groupUuids, total, nil
}

func (r *groupMemberStoreImpl) UpdateMuteUntil(ctx context.Context, groupUuid, userUuid string, muteUntil *time.Time) error {
	if err := r.db.WithContext(ctx).Model(&GroupMember{}).
		Where("group_uuid = ? AND user_uuid = ?", groupUuid, userUuid).
		Update("mute_until", muteUntil).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新群成员禁言时间 group_uuid=%s user_uuid=%s", groupUuid, userUuid)
	}
	return nil
}

// ==================== 工作单元 Unit of Work ====================

// UoW 群组领域事务工作单元
type UoW interface {
	WithTx(fn func(tx any) error) error
	RecordEvent(ctx context.Context, aggregateId, eventType string, payload []byte) error
	GroupStore() GroupStore
	GroupMemberStore() GroupMemberStore
}

type uowImpl struct {
	db          *gorm.DB
	groupStore  GroupStore
	memberStore GroupMemberStore
}

// NewUoW 创建工作单元实例
func NewUoW(db *gorm.DB) UoW {
	return &uowImpl{
		db:          db,
		groupStore:  NewGroupStore(db),
		memberStore: NewGroupMemberStore(db),
	}
}

func (u *uowImpl) GroupStore() GroupStore {
	return u.groupStore
}

func (u *uowImpl) GroupMemberStore() GroupMemberStore {
	return u.memberStore
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
