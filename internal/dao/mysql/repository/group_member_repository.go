package repository

import (
	"kama_chat_server/internal/model"

	"gorm.io/gorm"
)

type groupMemberRepository struct {
	db *gorm.DB
}

// NewGroupMemberRepository 创建群成员 Repository
func NewGroupMemberRepository(db *gorm.DB) GroupMemberRepository {
	return &groupMemberRepository{db: db}
}

// FindByGroupUuid 按群组ID查找所有成员
func (r *groupMemberRepository) FindByGroupUuid(groupUuid string) ([]model.GroupMember, error) {
	var members []model.GroupMember
	if err := r.db.Where("group_uuid = ?", groupUuid).Find(&members).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询群成员 group_uuid=%s", groupUuid)
	}
	return members, nil
}

// FindByUserUuid 按用户ID查找所在的群
func (r *groupMemberRepository) FindByUserUuid(userUuid string) ([]model.GroupMember, error) {
	var members []model.GroupMember
	if err := r.db.Where("user_uuid = ?", userUuid).Find(&members).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询用户所在群 user_uuid=%s", userUuid)
	}
	return members, nil
}

// FindByGroupAndUser 按群组ID和用户ID查找
func (r *groupMemberRepository) FindByGroupAndUser(groupUuid, userUuid string) (*model.GroupMember, error) {
	var member model.GroupMember
	if err := r.db.Where("group_uuid = ? AND user_uuid = ?", groupUuid, userUuid).First(&member).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询群成员 group_uuid=%s user_uuid=%s", groupUuid, userUuid)
	}
	return &member, nil
}

// FindMembersWithUserInfo 查询群成员详细信息（含用户资料）
func (r *groupMemberRepository) FindMembersWithUserInfo(groupUuid string) ([]GroupMemberWithUserInfo, error) {
	var members []GroupMemberWithUserInfo
	if err := r.db.Table("group_member").
		Select("user_info.uuid as user_id, user_info.nickname, user_info.avatar").
		Joins("LEFT JOIN user_info ON group_member.user_uuid = user_info.uuid").
		Where("group_member.group_uuid = ? AND group_member.deleted_at IS NULL", groupUuid).
		Scan(&members).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询群成员详情 group_uuid=%s", groupUuid)
	}
	return members, nil
}

// Create 创建群成员
func (r *groupMemberRepository) Create(member *model.GroupMember) error {
	if err := r.db.Create(member).Error; err != nil {
		return wrapDBError(err, "创建群成员")
	}
	return nil
}

// Delete 删除群成员
func (r *groupMemberRepository) Delete(groupUuid, userUuid string) error {
	if err := r.db.Where("group_uuid = ? AND user_uuid = ?", groupUuid, userUuid).Delete(&model.GroupMember{}).Error; err != nil {
		return wrapDBErrorf(err, "删除群成员 group_uuid=%s user_uuid=%s", groupUuid, userUuid)
	}
	return nil
}

// DeleteByGroupUuid 删除群组所有成员
func (r *groupMemberRepository) DeleteByGroupUuid(groupUuid string) error {
	if err := r.db.Where("group_uuid = ?", groupUuid).Delete(&model.GroupMember{}).Error; err != nil {
		return wrapDBErrorf(err, "删除群所有成员 group_uuid=%s", groupUuid)
	}
	return nil
}

// DeleteByUserUuids 批量删除群成员
func (r *groupMemberRepository) DeleteByUserUuids(groupUuid string, userUuids []string) error {
	if err := r.db.Where("group_uuid = ? AND user_uuid IN ?", groupUuid, userUuids).Delete(&model.GroupMember{}).Error; err != nil {
		return wrapDBErrorf(err, "批量删除群成员 group_uuid=%s", groupUuid)
	}
	return nil
}

// DeleteByGroupUuids 批量删除群组所有成员
func (r *groupMemberRepository) DeleteByGroupUuids(groupUuids []string) error {
	if len(groupUuids) == 0 {
		return nil
	}
	if err := r.db.Where("group_uuid IN ?", groupUuids).Delete(&model.GroupMember{}).Error; err != nil {
		return wrapDBError(err, "批量删除群所有成员")
	}
	return nil
}

// GetMemberIdsByGroupUuids 批量获取群组内的成员ID
func (r *groupMemberRepository) GetMemberIdsByGroupUuids(groupUuids []string) ([]string, error) {
	var members []string
	if len(groupUuids) == 0 {
		return members, nil
	}
	// pluck user_uuid distinct
	if err := r.db.Model(&model.GroupMember{}).Distinct("user_uuid").Where("group_uuid IN ?", groupUuids).Pluck("user_uuid", &members).Error; err != nil {
		return nil, wrapDBError(err, "批量查询群成员ID")
	}
	return members, nil
}
