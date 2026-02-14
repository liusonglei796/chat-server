// Package friendship 提供好友关系数据访问层的具体实现
//
// Friendship 表用于存储用户之间的好友关系，双向存储：
//   - A 和 B 是好友，则有两条记录 (A→B) 和 (B→A)
//
// 本文件实现 FriendshipRepository 接口
package friendship

import (
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/errorx"

	"gorm.io/gorm"
)

// friendshipRepository FriendshipRepository 接口的实现
type friendshipRepository struct {
	db *gorm.DB
}

// NewFriendshipRepository 创建 FriendshipRepository 实例
func NewFriendshipRepository(db *gorm.DB) *friendshipRepository {
	return &friendshipRepository{db: db}
}

// FindByUserIdAndFriendId 根据用户ID和好友ID查找好友关系记录
func (r *friendshipRepository) FindByUserIdAndFriendId(userId, friendId string) (*model.Friendship, error) {
	var fs model.Friendship
	if err := r.db.Where("user_id = ? AND friend_id = ?", userId, friendId).First(&fs).Error; err != nil {
		return nil, errorx.WrapDBErrorf(err, "查询好友关系 user_id=%s friend_id=%s", userId, friendId)
	}
	return &fs, nil
}

// IsFriend 判断两个用户是否互为好友
//
// 好友关系是双向的，需要同时存在两条记录且状态都为正常(status=0)
func (r *friendshipRepository) IsFriend(userId1, userId2 string) (bool, error) {
	var count int64
	if err := r.db.Model(&model.Friendship{}).
		Where("(user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)",
			userId1, userId2, userId2, userId1).
		Where("status = ?", 0).
		Count(&count).Error; err != nil {
		return false, errorx.WrapDBError(err, "查询好友关系")
	}
	return count == 2, nil
}

// FindFriendsByUserId 分页查询用户的好友列表
func (r *friendshipRepository) FindFriendsByUserId(userId string, page, pageSize int) ([]model.Friendship, int64, error) {
	var list []model.Friendship
	var total int64

	query := r.db.Model(&model.Friendship{}).
		Where("user_id = ?", userId)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "查询好友总数 user_id=%s", userId)
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, errorx.WrapDBErrorf(err, "查询好友列表 user_id=%s", userId)
	}

	return list, total, nil
}

// CreateFriendship 创建好友关系记录
func (r *friendshipRepository) CreateFriendship(fs *model.Friendship) error {
	if err := r.db.Create(fs).Error; err != nil {
		return errorx.WrapDBError(err, "创建好友关系")
	}
	return nil
}

// UpdateStatus 更新好友关系状态（正常/拉黑等）
func (r *friendshipRepository) UpdateStatus(userId, friendId string, status int8) error {
	if err := r.db.Model(&model.Friendship{}).
		Where("user_id = ? AND friend_id = ?", userId, friendId).
		Update("status", status).Error; err != nil {
		return errorx.WrapDBErrorf(err, "更新好友状态 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}

// SoftDelete 软删除好友关系（双向删除）
func (r *friendshipRepository) SoftDelete(userId, friendId string) error {
	if err := r.db.Where(
		"(user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)",
		userId, friendId, friendId, userId,
	).Delete(&model.Friendship{}).Error; err != nil {
		return errorx.WrapDBErrorf(err, "删除好友关系 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}

// SoftDeleteByUsers 批量软删除指定用户的所有好友关系
func (r *friendshipRepository) SoftDeleteByUsers(userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := r.db.Where("user_id IN ? OR friend_id IN ?", userUuids, userUuids).Delete(&model.Friendship{}).Error; err != nil {
		return errorx.WrapDBError(err, "批量删除好友关系")
	}
	return nil
}

// UpdateRemark 更新好友备注
func (r *friendshipRepository) UpdateRemark(userId, friendId, remark string) error {
	if err := r.db.Model(&model.Friendship{}).
		Where("user_id = ? AND friend_id = ?", userId, friendId).
		Update("remark", remark).Error; err != nil {
		return errorx.WrapDBErrorf(err, "更新好友备注 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}
