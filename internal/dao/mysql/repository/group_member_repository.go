// Package repository 提供数据访问层的具体实现
// 本文件实现 GroupMemberRepository 接口，处理群成员相关的数据库操作
package repository

import (
	"kama_chat_server/internal/model"

	"gorm.io/gorm"
)

// groupMemberRepository GroupMemberRepository 接口的实现
type groupMemberRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewGroupMemberRepository 创建 GroupMemberRepository 实例
func NewGroupMemberRepository(db *gorm.DB) GroupMemberRepository {
	return &groupMemberRepository{db: db}
}

// FindByGroupUuid 根据群组UUID查找所有成员
// groupUuid: 群组 UUID
// 返回: 群成员列表
func (r *groupMemberRepository) FindByGroupUuid(groupUuid string) ([]model.GroupMember, error) {
	var members []model.GroupMember
	if err := r.db.Where("group_uuid = ?", groupUuid).Find(&members).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询群成员 group_uuid=%s", groupUuid)
	}
	return members, nil
}

// FindByUserUuid 根据用户UUID查找加入的所有群组
// userUuid: 用户 UUID
// 返回: 用户加入的群成员记录列表
func (r *groupMemberRepository) FindByUserUuid(userUuid string) ([]model.GroupMember, error) {
	var members []model.GroupMember
	if err := r.db.Where("user_uuid = ?", userUuid).Find(&members).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询用户所在群 user_uuid=%s", userUuid)
	}
	return members, nil
}

// FindByGroupAndUser 根据群组和用户查找成员关系
// 用于检查用户是否已在群中
func (r *groupMemberRepository) FindByGroupAndUser(groupUuid, userUuid string) (*model.GroupMember, error) {
	var member model.GroupMember
	if err := r.db.Where("group_uuid = ? AND user_uuid = ?", groupUuid, userUuid).First(&member).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询群成员 group_uuid=%s user_uuid=%s", groupUuid, userUuid)
	}
	return &member, nil
}

// FindMembersWithUserInfo 查询群成员详细信息（包含用户基本资料）
// 通过 JOIN 查询关联用户表获取昵称和头像
// groupUuid: 群组 UUID
// 返回: 带用户信息的群成员列表
func (r *groupMemberRepository) FindMembersWithUserInfo(groupUuid string) ([]GroupMemberWithUserInfo, error) {
	var members []GroupMemberWithUserInfo
	// 使用 LEFT JOIN 关联 user_info 表
	if err := r.db.Table("group_member").
		Select("user_info.uuid as user_id, user_info.nickname, user_info.avatar").
		Joins("LEFT JOIN user_info ON group_member.user_uuid = user_info.uuid").
		Where("group_member.group_uuid = ? AND group_member.deleted_at IS NULL", groupUuid).
		Scan(&members).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询群成员详情 group_uuid=%s", groupUuid)
	}
	return members, nil
}

// Create 添加群成员
func (r *groupMemberRepository) Create(member *model.GroupMember) error {
	if err := r.db.Create(member).Error; err != nil {
		return wrapDBError(err, "创建群成员")
	}
	return nil
}

// Delete 删除单个群成员
func (r *groupMemberRepository) Delete(groupUuid, userUuid string) error {
	if err := r.db.Where("group_uuid = ? AND user_uuid = ?", groupUuid, userUuid).Delete(&model.GroupMember{}).Error; err != nil {
		return wrapDBErrorf(err, "删除群成员 group_uuid=%s user_uuid=%s", groupUuid, userUuid)
	}
	return nil
}

// DeleteByGroupUuid 删除群组的所有成员
// 用于解散群组时清理成员数据
func (r *groupMemberRepository) DeleteByGroupUuid(groupUuid string) error {
	if err := r.db.Where("group_uuid = ?", groupUuid).Delete(&model.GroupMember{}).Error; err != nil {
		return wrapDBErrorf(err, "删除群所有成员 group_uuid=%s", groupUuid)
	}
	return nil
}

// DeleteByUserUuids 批量删除指定用户（踢人）
func (r *groupMemberRepository) DeleteByUserUuids(groupUuid string, userUuids []string) error {
	if err := r.db.Where("group_uuid = ? AND user_uuid IN ?", groupUuid, userUuids).Delete(&model.GroupMember{}).Error; err != nil {
		return wrapDBErrorf(err, "批量删除群成员 group_uuid=%s", groupUuid)
	}
	return nil
}

// DeleteByGroupUuids 批量删除多个群组的所有成员
// 用于批量删除群组时清理成员数据
func (r *groupMemberRepository) DeleteByGroupUuids(groupUuids []string) error {
	if len(groupUuids) == 0 {
		return nil
	}
	if err := r.db.Where("group_uuid IN ?", groupUuids).Delete(&model.GroupMember{}).Error; err != nil {
		return wrapDBError(err, "批量删除群所有成员")
	}
	return nil
}

// GetMemberIdsByGroupUuids 获取多个群组的所有成员UUID（去重）
// 用于批量操作时获取受影响的用户
func (r *groupMemberRepository) GetMemberIdsByGroupUuids(groupUuids []string) ([]string, error) {
	var members []string
	if len(groupUuids) == 0 {
		return members, nil
	}
	// Distinct: 去重，避免用户在多个群中时重复
	// Pluck: 只获取指定字段的值
	if err := r.db.Model(&model.GroupMember{}).Distinct("user_uuid").Where("group_uuid IN ?", groupUuids).Pluck("user_uuid", &members).Error; err != nil {
		return nil, wrapDBError(err, "批量查询群成员ID")
	}
	return members, nil
}
