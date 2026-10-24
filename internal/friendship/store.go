package friendship

import (
	"context"

	"gorm.io/gorm"

	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/dberr"
)

// Store 好友关系数据访问接口
type Store interface {
	FindByUserIdAndFriendId(ctx context.Context, userId, friendId string) (*Friendship, error)
	IsFriend(ctx context.Context, userId1, userId2 string) (bool, error)
	FindFriendsByUserId(ctx context.Context, userId string, page, pageSize int) ([]Friendship, int64, error)
	CreateFriendship(ctx context.Context, fs *Friendship) error
	UpdateStatus(ctx context.Context, userId, friendId string, status int8) error
	UpdateStatusWithExpected(ctx context.Context, userId, friendId string, expectedStatus, targetStatus int8, expectedVersion int64) (bool, error)
	SoftDelete(ctx context.Context, userId, friendId string) error
	SoftDeleteByUsers(ctx context.Context, userUuids []string) error
	UpdateRemark(ctx context.Context, userId, friendId, remark string) error
}

type storeImpl struct {
	db *gorm.DB
}

// NewStore 创建 Store 实例
func NewStore(db *gorm.DB) Store {
	return &storeImpl{db: db}
}

// FindByUserIdAndFriendId 根据用户ID和好友ID查找好友关系记录
func (r *storeImpl) FindByUserIdAndFriendId(ctx context.Context, userId, friendId string) (*Friendship, error) {
	var fs Friendship
	if err := database.GetDB(ctx, r.db).Where("user_id = ? AND friend_id = ?", userId, friendId).First(&fs).Error; err != nil {
		return nil, dberr.WrapDBErrorf(err, "查询好友关系 user_id=%s friend_id=%s", userId, friendId)
	}
	return &fs, nil
}

// IsFriend 判断两个用户是否互为好友
func (r *storeImpl) IsFriend(ctx context.Context, userId1, userId2 string) (bool, error) {
	var count int64
	if err := database.GetDB(ctx, r.db).Model(&Friendship{}).
		Where("((user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)) AND status = ?",
			userId1, userId2, userId2, userId1, 0).
		Count(&count).Error; err != nil {
		return false, dberr.WrapDBError(err, "查询好友关系")
	}
	return count == 2, nil
}

// FindFriendsByUserId 分页查询用户的好友列表
func (r *storeImpl) FindFriendsByUserId(ctx context.Context, userId string, page, pageSize int) ([]Friendship, int64, error) {
	var list []Friendship
	var total int64

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := database.GetDB(ctx, r.db).Model(&Friendship{}).
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
func (r *storeImpl) CreateFriendship(ctx context.Context, fs *Friendship) error {
	if err := database.GetDB(ctx, r.db).Create(fs).Error; err != nil {
		return dberr.WrapDBError(err, "创建好友关系")
	}
	return nil
}

// UpdateStatus 更新好友关系状态
func (r *storeImpl) UpdateStatus(ctx context.Context, userId, friendId string, status int8) error {
	if err := database.GetDB(ctx, r.db).Model(&Friendship{}).
		Where("user_id = ? AND friend_id = ?", userId, friendId).
		Update("status", status).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新好友状态 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}

// UpdateStatusWithExpected 原子比较并更新好友关系状态与版本号（状态机与版本号双重乐观锁 CAS）
func (r *storeImpl) UpdateStatusWithExpected(ctx context.Context, userId, friendId string, expectedStatus, targetStatus int8, expectedVersion int64) (bool, error) {
	updates := map[string]interface{}{
		"status":  targetStatus,
		"version": gorm.Expr("version + 1"),
	}

	query := database.GetDB(ctx, r.db).Model(&Friendship{}).
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

// SoftDelete 删除单向好友关系（物理删除 userId -> friendId 单向记录，释放唯一索引槽位，允许重新加回）
func (r *storeImpl) SoftDelete(ctx context.Context, userId, friendId string) error {
	if err := database.GetDB(ctx, r.db).Where(
		"user_id = ? AND friend_id = ?",
		userId, friendId,
	).Delete(&Friendship{}).Error; err != nil {
		return dberr.WrapDBErrorf(err, "删除单向好友关系 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}

// SoftDeleteByUsers 批量删除指定用户的所有好友关系
func (r *storeImpl) SoftDeleteByUsers(ctx context.Context, userUuids []string) error {
	if len(userUuids) == 0 {
		return nil
	}
	if err := database.GetDB(ctx, r.db).Where("user_id IN ? OR friend_id IN ?", userUuids, userUuids).Delete(&Friendship{}).Error; err != nil {
		return dberr.WrapDBError(err, "批量删除好友关系")
	}
	return nil
}

// UpdateRemark 更新好友备注
func (r *storeImpl) UpdateRemark(ctx context.Context, userId, friendId, remark string) error {
	if err := database.GetDB(ctx, r.db).Model(&Friendship{}).
		Where("user_id = ? AND friend_id = ?", userId, friendId).
		Update("remark", remark).Error; err != nil {
		return dberr.WrapDBErrorf(err, "更新好友备注 user_id=%s friend_id=%s", userId, friendId)
	}
	return nil
}
