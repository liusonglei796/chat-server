package group

import (
	"context"
	"time"

	"gorm.io/gorm"

	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/pkg/enum/group/group_status"
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
	IncrementMemberCountWithLimit(ctx context.Context, uuid string, maxLimit int) (bool, error)
	DecrementMemberCountBy(ctx context.Context, uuid string, count int) error
	SoftDeleteByUuids(ctx context.Context, uuids []string) error
	DismissGroupWithCAS(ctx context.Context, uuid, ownerId string) (bool, error)
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
	if err := database.GetDB(ctx, r.db).First(&g, "uuid = ?", uuid).Error; err != nil {
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

	if err := database.GetDB(ctx, r.db).Model(&GroupInfo{}).Where("owner_id = ?", ownerId).Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计群组数量 owner_id=%s", ownerId)
	}

	offset := (page - 1) * pageSize
	if err := database.GetDB(ctx, r.db).Where("owner_id = ?", ownerId).
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
	if err := database.GetDB(ctx, r.db).Where("owner_id = ? AND name = ? AND status = 0", ownerId, name).First(&g).Error; err != nil {
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

	if err := database.GetDB(ctx, r.db).Unscoped().Model(&GroupInfo{}).Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBError(err, "查询群组总数")
	}

	if err := database.GetDB(ctx, r.db).Unscoped().Model(&GroupInfo{}).Offset(offset).Limit(pageSize).Find(&groups).Error; err != nil {
		return nil, 0, dberr.WrapDBError(err, "分页查询群组")
	}

	return groups, total, nil
}

func (r *groupStoreImpl) FindByUuids(ctx context.Context, uuids []string) ([]GroupInfo, error) {
	var groups []GroupInfo
	if err := database.GetDB(ctx, r.db).Where("uuid IN ?", uuids).Find(&groups).Error; err != nil {
		return nil, dberr.WrapDBError(err, "批量查询群组")
	}
	return groups, nil
}

func (r *groupStoreImpl) CreateGroup(ctx context.Context, group *GroupInfo) error {
	if err := database.GetDB(ctx, r.db).Create(group).Error; err != nil {
		return dberr.WrapDBError(err, "创建群组")
	}
	return nil
}

func (r *groupStoreImpl) Update(ctx context.Context, group *GroupInfo) error {
	if err := database.GetDB(ctx, r.db).Save(group).Error; err != nil {
		return dberr.WrapDBError(err, "更新群组")
	}
	return nil
}

func (r *groupStoreImpl) UpdateWithVersion(ctx context.Context, uuid string, expectedVersion int32, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	updates["version"] = gorm.Expr("version + ?", 1)
	result := database.GetDB(ctx, r.db).Model(&GroupInfo{}).
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
	if err := database.GetDB(ctx, r.db).Model(&GroupInfo{}).Where("uuid IN ?", uuids).Update("status", status).Error; err != nil {
		return dberr.WrapDBError(err, "批量更新群组状态")
	}
	return nil
}

func (r *groupStoreImpl) IncrementMemberCount(ctx context.Context, uuid string) error {
	if err := database.GetDB(ctx, r.db).Model(&GroupInfo{}).Where("uuid = ?", uuid).UpdateColumn("member_cnt", gorm.Expr("member_cnt + ?", 1)).Error; err != nil {
		return dberr.WrapDBErrorf(err, "增加群成员数 uuid=%s", uuid)
	}
	return nil
}

func (r *groupStoreImpl) IncrementMemberCountWithLimit(ctx context.Context, uuid string, maxLimit int) (bool, error) {
	result := database.GetDB(ctx, r.db).Model(&GroupInfo{}).
		Where("uuid = ? AND member_cnt < ?", uuid, maxLimit).
		UpdateColumn("member_cnt", gorm.Expr("member_cnt + ?", 1))
	if result.Error != nil {
		return false, dberr.WrapDBErrorf(result.Error, "增加群成员数(限额) uuid=%s max=%d", uuid, maxLimit)
	}
	return result.RowsAffected > 0, nil
}

func (r *groupStoreImpl) DecrementMemberCountBy(ctx context.Context, uuid string, count int) error {
	if count <= 0 {
		return nil
	}
	if err := database.GetDB(ctx, r.db).Model(&GroupInfo{}).Where("uuid = ?", uuid).UpdateColumn("member_cnt", gorm.Expr("GREATEST(0, member_cnt - ?)", count)).Error; err != nil {
		return dberr.WrapDBErrorf(err, "减少群成员数 uuid=%s count=%d", uuid, count)
	}
	return nil
}

func (r *groupStoreImpl) SoftDeleteByUuids(ctx context.Context, uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := database.GetDB(ctx, r.db).Unscoped().Where("uuid IN ?", uuids).Delete(&GroupInfo{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除群组")
	}
	return nil
}

func (r *groupStoreImpl) DismissGroupWithCAS(ctx context.Context, uuid, ownerId string) (bool, error) {
	result := database.GetDB(ctx, r.db).Model(&GroupInfo{}).
		Where("uuid = ? AND owner_id = ? AND status = ?", uuid, ownerId, group_status.NORMAL).
		Updates(map[string]interface{}{
			"status":     group_status.DISMISS,
			"deleted_at": gorm.Expr("NOW()"),
			"version":    gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return false, dberr.WrapDBErrorf(result.Error, "CAS解散群组 uuid=%s owner_id=%s", uuid, ownerId)
	}
	return result.RowsAffected > 0, nil
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
	DeleteMemberIfNotOwner(ctx context.Context, groupUuid, userUuid string) (bool, error)
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
	if err := database.GetDB(ctx, r.db).Where("group_uuid = ?", groupUuid).Find(&members).Error; err != nil {
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

	if err := database.GetDB(ctx, r.db).Table("group_member").
		Where("group_uuid = ?", groupUuid).
		Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计群成员数量 group_uuid=%s", groupUuid)
	}

	offset := (page - 1) * pageSize
	if err := database.GetDB(ctx, r.db).Table("group_member").
		Select("user_info.uuid as user_id, user_info.nickname, user_info.avatar").
		Joins("LEFT JOIN user_info ON group_member.user_uuid = user_info.uuid").
		Where("group_member.group_uuid = ?", groupUuid).
		Offset(offset).
		Limit(pageSize).
		Scan(&members).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询群成员详情 group_uuid=%s", groupUuid)
	}
	return members, total, nil
}

func (r *groupMemberStoreImpl) FindByGroupAndUser(ctx context.Context, groupUuid, userUuid string) (*GroupMember, error) {
	var member GroupMember
	if err := database.GetDB(ctx, r.db).Where("group_uuid = ? AND user_uuid = ?", groupUuid, userUuid).First(&member).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询群成员 group_uuid=%s user_uuid=%s", groupUuid, userUuid)
	}
	return &member, nil
}

func (r *groupMemberStoreImpl) CreateGroupMember(ctx context.Context, member *GroupMember) error {
	if err := database.GetDB(ctx, r.db).Create(member).Error; err != nil {
		return dberr.WrapDBError(err, "创建群成员")
	}
	return nil
}

func (r *groupMemberStoreImpl) DeleteByGroupUuid(ctx context.Context, groupUuid string) error {
	if err := database.GetDB(ctx, r.db).Where("group_uuid = ?", groupUuid).Delete(&GroupMember{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "删除群所有成员 group_uuid=%s", groupUuid)
	}
	return nil
}

func (r *groupMemberStoreImpl) DeleteByUserUuids(ctx context.Context, groupUuid string, userUuids []string) (int64, error) {
	if len(userUuids) == 0 {
		return 0, nil
	}
	res := database.GetDB(ctx, r.db).Where("group_uuid = ? AND user_uuid IN ?", groupUuid, userUuids).Delete(&GroupMember{})
	if err := res.Error; err != nil {
		return 0, dberr.WrapDBErrorf(err, "批量删除群成员 group_uuid=%s", groupUuid)
	}
	return res.RowsAffected, nil
}

func (r *groupMemberStoreImpl) DeleteByGroupUuids(ctx context.Context, groupUuids []string) error {
	if len(groupUuids) == 0 {
		return nil
	}
	if err := database.GetDB(ctx, r.db).Where("group_uuid IN ?", groupUuids).Delete(&GroupMember{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除群所有成员")
	}
	return nil
}

func (r *groupMemberStoreImpl) GetMemberIdsByGroupUuids(ctx context.Context, groupUuids []string) ([]string, error) {
	var members []string
	if len(groupUuids) == 0 {
		return members, nil
	}
	if err := database.GetDB(ctx, r.db).Model(&GroupMember{}).Distinct("user_uuid").Where("group_uuid IN ?", groupUuids).Pluck("user_uuid", &members).Error; err != nil {
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

	if err := database.GetDB(ctx, r.db).Model(&GroupMember{}).
		Where("user_uuid = ?", userUuid).
		Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计用户群组数量 user_uuid=%s", userUuid)
	}

	offset := (page - 1) * pageSize
	var groupUuids []string
	if err := database.GetDB(ctx, r.db).Model(&GroupMember{}).
		Where("user_uuid = ?", userUuid).
		Offset(offset).
		Limit(pageSize).
		Pluck("group_uuid", &groupUuids).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询用户群组 user_uuid=%s", userUuid)
	}
	return groupUuids, total, nil
}

func (r *groupMemberStoreImpl) DeleteMemberIfNotOwner(ctx context.Context, groupUuid, userUuid string) (bool, error) {
	result := database.GetDB(ctx, r.db).
		Where("group_uuid = ? AND user_uuid = ? AND role != ?", groupUuid, userUuid, 3).
		Delete(&GroupMember{})
	if result.Error != nil {
		return false, dberr.WrapDBErrorf(result.Error, "原子退群 group_uuid=%s user_uuid=%s", groupUuid, userUuid)
	}
	return result.RowsAffected > 0, nil
}

func (r *groupMemberStoreImpl) UpdateMuteUntil(ctx context.Context, groupUuid, userUuid string, muteUntil *time.Time) error {
	query := database.GetDB(ctx, r.db).Model(&GroupMember{}).
		Where("group_uuid = ? AND user_uuid = ? AND role != ?", groupUuid, userUuid, 3)
	if muteUntil != nil {
		query = query.Where("mute_until IS NULL OR mute_until < ?", *muteUntil)
	}
	if err := query.Update("mute_until", muteUntil).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新群成员禁言时间 group_uuid=%s user_uuid=%s", groupUuid, userUuid)
	}
	return nil
}
