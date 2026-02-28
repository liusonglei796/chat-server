// Package friendship 提供好友关系数据访问层的具体实现
//
// Friendship 表用于存储用户之间的好友关系，双向存储：
//   - A 和 B 是好友，则有两条记录 (A→B) 和 (B→A)
//
// 本文件实现 FriendshipRepository 接口
package friendship

import (
	"context"
	"kama_chat_server/internal/dao/mysql/dberr"
	"kama_chat_server/internal/model"

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
func (r *friendshipRepository) FindByUserIdAndFriendId(ctx context.Context, userId, friendId string) (*model.Friendship, error) {
	var fs model.Friendship
	if err := r.db.WithContext(ctx).Where("user_id = ? AND friend_id = ?", userId, friendId).First(&fs).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询好友关系 user_id=%s friend_id=%s", userId, friendId)
	}
	return &fs, nil
}

// IsFriend 判断两个用户是否互为好友
//
// 查询两人之间的双向关系记录（(A,B) 或 (B,A)），且状态必须为正常(status=0)
func (r *friendshipRepository) IsFriend(ctx context.Context, userId1, userId2 string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("((user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)) AND status = ?",
			userId1, userId2, userId2, userId1, 0).
		Count(&count).Error; err != nil {
		return false, dberr.WrapDBError(err, "查询好友关系")
	}
	return count == 2, nil
}

// FindFriendsByUserId 分页查询用户的好友列表
func (r *friendshipRepository) FindFriendsByUserId(ctx context.Context, userId string, page, pageSize int) ([]model.Friendship, int64, error) {
	var list []model.Friendship
	var total int64

	// 校验分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("user_id = ?", userId)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "查询好友总数 user_id=%s", userId)
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, dberr.WrapDBErrorf(err, "查询好友列表 user_id=%s", userId)
	}

	return list, total, nil
}

// CreateFriendship 创建好友关系记录
func (r *friendshipRepository) CreateFriendship(ctx context.Context, fs *model.Friendship) error {
	if err := r.db.WithContext(ctx).Create(fs).Error; err != nil {
		return dberr.WrapDBError(err, "创建好友关系")
	}
	return nil
}

// UpdateStatus 更新好友关系状态（正常/拉黑等）
func (r *friendshipRepository) UpdateStatus(ctx context.Context, userId, friendId string, status int8) error {
	if err := r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("user_id = ? AND friend_id = ?", userId, friendId).
		Update("status", status).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新好友状态 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}

// SoftDelete 软删除好友关系（双向删除）
func (r *friendshipRepository) SoftDelete(ctx context.Context, userId, friendId string) error {
	if err := r.db.WithContext(ctx).Where(
		"(user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)",
		userId, friendId, friendId, userId,
	).Delete(&model.Friendship{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "删除好友关系 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}

// SoftDeleteByUsers 批量软删除指定用户的所有好友关系
func (r *friendshipRepository) SoftDeleteByUsers(ctx context.Context, userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Where("user_id IN ? OR friend_id IN ?", userUuids, userUuids).Delete(&model.Friendship{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除好友关系")
	}
	return nil
}

// UpdateRemark 更新好友备注
func (r *friendshipRepository) UpdateRemark(ctx context.Context, userId, friendId, remark string) error {
	if err := r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("user_id = ? AND friend_id = ?", userId, friendId).
		Update("remark", remark).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新好友备注 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}
