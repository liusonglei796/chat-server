// Package friendship 提供好友关系数据访问层的具体实现
//
// Friendship 表用于存储用户之间的好友关系，双向存储：
//   - A 和 B 是好友，则有两条记录 (A→B) 和 (B→A)
//
// 本文件实现 FriendshipStore 接口
package friendship

import (
	"context"
	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/model"

	"gorm.io/gorm"
)

// friendshipStore FriendshipStore 接口的实现
type friendshipStore struct {
	db *gorm.DB
}

// NewFriendshipStore 创建 FriendshipStore 实例
func NewFriendshipStore(db *gorm.DB) *friendshipStore {
	return &friendshipStore{db: db}
}

// FindByUserIdAndFriendId 根据用户ID和好友ID查找好友关系记录
func (r *friendshipStore) FindByUserIdAndFriendId(ctx context.Context, userId, friendId string) (*model.Friendship, error) {
	var fs model.Friendship
	if err := r.db.WithContext(ctx).Where("user_id = ? AND friend_id = ?", userId, friendId).First(&fs).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询好友关系 user_id=%s friend_id=%s", userId, friendId)
	}
	return &fs, nil
}

// IsFriend 判断两个用户是否互为好友
//
// 查询两人之间的双向关系记录（(A,B) 或 (B,A)），且状态必须为正常(status=0)
func (r *friendshipStore) IsFriend(ctx context.Context, userId1, userId2 string) (bool, error) {
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
func (r *friendshipStore) FindFriendsByUserId(ctx context.Context, userId string, page, pageSize int) ([]model.Friendship, int64, error) {
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
func (r *friendshipStore) CreateFriendship(ctx context.Context, fs *model.Friendship) error {
	if err := r.db.WithContext(ctx).Create(fs).Error; err != nil {
		return dberr.WrapDBError(err, "创建好友关系")
	}
	return nil
}

// UpdateStatus 更新好友关系状态（正常/拉黑等）
func (r *friendshipStore) UpdateStatus(ctx context.Context, userId, friendId string, status int8) error {
	if err := r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("user_id = ? AND friend_id = ?", userId, friendId).
		Update("status", status).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新好友状态 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}

// UpdateStatusWithExpected 原子比较并更新好友关系状态与版本号（状态机与版本号双重乐观锁 CAS）
// 来源: BlackFriend / UnblackFriend 状态跃迁
// 用于: 并发防重、ABA 阻断与副作用阻断，仅当当前状态为 expectedStatus 且版本号为 expectedVersion 时更新，依赖 RowsAffected > 0 判定成功
func (r *friendshipStore) UpdateStatusWithExpected(ctx context.Context, userId, friendId string, expectedStatus, targetStatus int8, expectedVersion int64) (bool, error) {
	// Step 1.1: 构建更新载荷与递增版本表达式
	// 来源: 目标状态 targetStatus 与自增版本需求
	// 用于: 原子跃迁状态并使版本单调递增
	updates := map[string]interface{}{
		"status":  targetStatus,
		"version": gorm.Expr("version + 1"),
	}

	// Step 1.2: 执行带状态与版本双重约束的原子条件更新
	// 来源: userId, friendId, expectedStatus, expectedVersion
	// 用于: 防止状态倒退与 ABA 穿透，产出影响行数校验结果
	query := r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("user_id = ? AND friend_id = ? AND status = ?", userId, friendId, expectedStatus)
	if expectedVersion > 0 {
		query = query.Where("version = ?", expectedVersion)
	}

	result := query.Updates(updates)
	if result.Error != nil {
		return false, dberr.WrapDBErrorf(result.Error, "状态机与版本号更新好友状态 user_id=%s friend_id=%s expected=%d target=%d version=%d", userId, friendId, expectedStatus, targetStatus, expectedVersion)
	}
	return result.RowsAffected > 0, nil
}

// SoftDelete 删除好友关系（双向删除，采用物理删除释放唯一索引槽位，允许重新加好友）
func (r *friendshipStore) SoftDelete(ctx context.Context, userId, friendId string) error {
	if err := r.db.WithContext(ctx).Unscoped().Where(
		"(user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)",
		userId, friendId, friendId, userId,
	).Delete(&model.Friendship{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "删除好友关系 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}

// SoftDeleteByUsers 批量删除指定用户的所有好友关系（物理删除释放唯一索引槽位）
func (r *friendshipStore) SoftDeleteByUsers(ctx context.Context, userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Unscoped().Where("user_id IN ? OR friend_id IN ?", userUuids, userUuids).Delete(&model.Friendship{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除好友关系")
	}
	return nil
}

// UpdateRemark 更新好友备注
func (r *friendshipStore) UpdateRemark(ctx context.Context, userId, friendId, remark string) error {
	if err := r.db.WithContext(ctx).Model(&model.Friendship{}).
		Where("user_id = ? AND friend_id = ?", userId, friendId).
		Update("remark", remark).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新好友备注 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}
