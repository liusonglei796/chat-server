// Package member 提供群成员相关数据访问层的具体实现
// 本文件实现 GroupMemberRepository 接口，处理群成员相关的数据库操作
package member

import (
	"kama_chat_server/internal/dao/mysql/dberr"
	"kama_chat_server/internal/model"
	"time"

	"gorm.io/gorm"
)

// groupMemberRepository GroupMemberRepository 接口的实现
type groupMemberRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewGroupMemberRepository 创建 GroupMemberRepository 实例
func NewGroupMemberRepository(db *gorm.DB) *groupMemberRepository {
	return &groupMemberRepository{db: db}
}

// FindByGroupUuid 根据群组UUID查找所有成员
// groupUuid: 群组 UUID
// 返回: 群成员列表
func (r *groupMemberRepository) FindByGroupUuid(groupUuid string) ([]model.GroupMember, error) {
	var members []model.GroupMember
	if err := r.db.Where("group_uuid = ?", groupUuid).Find(&members).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询群成员 group_uuid=%s", groupUuid)
	}
	return members, nil
}

// FindMembersWithUserInfoPaged 分页查询群成员详细信息
// groupUuid: 群组 UUID
// page: 页码（从1开始）
// pageSize: 每页数量
// 返回: 带用户信息的群成员列表、总数和错误
func (r *groupMemberRepository) FindMembersWithUserInfoPaged(groupUuid string, page, pageSize int) ([]model.GroupMemberWithUserInfo, int64, error) {
	var members []model.GroupMemberWithUserInfo
	var total int64

	// 校验分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 统计总数
	if err := r.db.Table("group_member").
		Where("group_uuid = ? AND deleted_at IS NULL", groupUuid).
		Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计群成员数量 group_uuid=%s", groupUuid)
	}

	// 计算偏移量并分页查询
	offset := (page - 1) * pageSize
	if err := r.db.Table("group_member").
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

// FindByGroupAndUser 根据群组UUID和用户UUID查找成员
// 用于权限校验：检查用户是否是群成员及其角色
func (r *groupMemberRepository) FindByGroupAndUser(groupUuid, userUuid string) (*model.GroupMember, error) {
	var member model.GroupMember
	if err := r.db.Where("group_uuid = ? AND user_uuid = ?", groupUuid, userUuid).First(&member).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询群成员 group_uuid=%s user_uuid=%s", groupUuid, userUuid)
	}
	return &member, nil
}

// CreateGroupMember 添加群成员
func (r *groupMemberRepository) CreateGroupMember(member *model.GroupMember) error {
	if err := r.db.Create(member).Error; err != nil {
		return dberr.WrapDBError(err, "创建群成员")
	}
	return nil
}

// DeleteByGroupUuid 删除群组的所有成员
// 用于解散群组时清理成员数据
func (r *groupMemberRepository) DeleteByGroupUuid(groupUuid string) error {
	if err := r.db.Where("group_uuid = ?", groupUuid).Delete(&model.GroupMember{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "删除群所有成员 group_uuid=%s", groupUuid)
	}
	return nil
}

// DeleteByUserUuids 批量删除指定用户（踢人）
func (r *groupMemberRepository) DeleteByUserUuids(groupUuid string, userUuids []string) error {
	if err := r.db.Where("group_uuid = ? AND user_uuid IN ?", groupUuid, userUuids).Delete(&model.GroupMember{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "批量删除群成员 group_uuid=%s", groupUuid)
	}
	return nil
}

// DeleteByGroupUuids [管理员] 批量删除多个群组的所有成员
// 用于批量删除群组时清理成员数据
func (r *groupMemberRepository) DeleteByGroupUuids(groupUuids []string) error {
	if len(groupUuids) == 0 {
		return nil
	}
	if err := r.db.Where("group_uuid IN ?", groupUuids).Delete(&model.GroupMember{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除群所有成员")
	}
	return nil
}

// GetMemberIdsByGroupUuids [管理员] 获取多个群组的所有成员UUID（去重）
// 用于批量操作时获取受影响的用户
func (r *groupMemberRepository) GetMemberIdsByGroupUuids(groupUuids []string) ([]string, error) {
	var members []string
	if len(groupUuids) == 0 {
		return members, nil
	}
	// Distinct: 去重，避免用户在多个群中时重复
	// Pluck: 只获取指定字段的值
	if err := r.db.Model(&model.GroupMember{}).Distinct("user_uuid").Where("group_uuid IN ?", groupUuids).Pluck("user_uuid", &members).Error; err != nil {
		return nil, dberr.WrapDBError(err, "批量查询群成员ID")
	}
	return members, nil
}

// FindGroupUuidsByUserPaged 根据用户UUID分页查找其加入的群组UUID
func (r *groupMemberRepository) FindGroupUuidsByUserPaged(userUuid string, page, pageSize int) ([]string, int64, error) {
	var total int64

	// 校验分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 统计总数
	if err := r.db.Model(&model.GroupMember{}).
		Where("user_uuid = ?", userUuid).
		Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "统计用户群组数量 user_uuid=%s", userUuid)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	var groupUuids []string
	if err := r.db.Model(&model.GroupMember{}).
		Where("user_uuid = ?", userUuid).
		Offset(offset).
		Limit(pageSize).
		Pluck("group_uuid", &groupUuids).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "分页查询用户群组 user_uuid=%s", userUuid)
	}
	return groupUuids, total, nil
}

// UpdateMuteUntil 更新群成员禁言截止时间
// muteUntil 为 nil 表示取消禁言
func (r *groupMemberRepository) UpdateMuteUntil(groupUuid, userUuid string, muteUntil *time.Time) error {
	if err := r.db.Model(&model.GroupMember{}).
		Where("group_uuid = ? AND user_uuid = ?", groupUuid, userUuid).
		Update("mute_until", muteUntil).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新群成员禁言时间 group_uuid=%s user_uuid=%s", groupUuid, userUuid)
	}
	return nil
}
