// Package repository 提供数据访问层的具体实现
// 本文件实现 UserRepository 接口，处理用户相关的数据库操作
package repository

import (
	"kama_chat_server/internal/model"

	"gorm.io/gorm"
)

// userRepository UserRepository 接口的实现
// 使用 GORM 进行数据库操作
type userRepository struct {
	db *gorm.DB // GORM 数据库实例
}

// NewUserRepository 创建 UserRepository 实例
// db: GORM 数据库实例
// 返回: UserRepository 接口实现
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

// FindByUuid 根据 UUID 查找用户
// uuid: 用户唯一标识
// 返回: 用户信息和错误
func (r *userRepository) FindByUuid(uuid string) (*model.UserInfo, error) {
	var user model.UserInfo
	// GORM First 方法：查找第一条匹配记录
	// 如果未找到会返回 ErrRecordNotFound
	if err := r.db.First(&user, "uuid = ?", uuid).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询用户 uuid=%s", uuid)
	}
	return &user, nil
}

// FindByTelephone 根据手机号查找用户
// 用于登录验证和注册检查
// telephone: 手机号码
// 返回: 用户信息和错误
func (r *userRepository) FindByTelephone(telephone string) (*model.UserInfo, error) {
	var user model.UserInfo
	if err := r.db.First(&user, "telephone = ?", telephone).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询用户 telephone=%s", telephone)
	}
	return &user, nil
}

// FindAllExcept 查找除指定用户外的所有用户
// 用于获取用户列表（排除当前用户自己）
// excludeUuid: 要排除的用户 UUID
// 返回: 用户列表和错误
func (r *userRepository) FindAllExcept(excludeUuid string) ([]model.UserInfo, error) {
	var users []model.UserInfo
	// Unscoped: 包含软删除的记录（如果需要排除软删除，去掉 Unscoped）
	if err := r.db.Unscoped().Where("uuid != ?", excludeUuid).Find(&users).Error; err != nil {
		return nil, wrapDBError(err, "查询用户列表")
	}
	return users, nil
}

// FindByUuids 批量根据 UUID 查找用户
// 用于批量获取用户信息
// uuids: UUID 列表
// 返回: 用户列表和错误
func (r *userRepository) FindByUuids(uuids []string) ([]model.UserInfo, error) {
	var users []model.UserInfo
	// IN 查询：UUID IN ('uuid1', 'uuid2', ...)
	if err := r.db.Where("uuid IN ?", uuids).Find(&users).Error; err != nil {
		return nil, wrapDBError(err, "批量查询用户")
	}
	return users, nil
}

// Create 创建新用户
// 密码会在 BeforeSave Hook 中自动加密
// user: 用户信息结构体
// 返回: 操作错误
func (r *userRepository) Create(user *model.UserInfo) error {
	if err := r.db.Create(user).Error; err != nil {
		return wrapDBError(err, "创建用户")
	}
	return nil
}

// UpdateUserInfo 更新用户信息
// 使用 Save 方法更新所有字段
// user: 包含更新后数据的用户结构体
// 返回: 操作错误
func (r *userRepository) UpdateUserInfo(user *model.UserInfo) error {
	// Save: 保存所有字段，如果主键不存在则创建
	if err := r.db.Save(user).Error; err != nil {
		return wrapDBError(err, "更新用户信息")
	}
	return nil
}

// UpdateUserStatusByUuids 批量更新用户状态
// 用于管理员启用/禁用用户
// uuids: 用户 UUID 列表
// status: 新状态（0=正常, 1=禁用）
// 返回: 操作错误
func (r *userRepository) UpdateUserStatusByUuids(uuids []string, status int8) error {
	if len(uuids) == 0 {
		return nil // 空列表直接返回
	}
	// Model: 指定操作的表/模型
	// Update: 只更新指定字段
	if err := r.db.Model(&model.UserInfo{}).Where("uuid IN ?", uuids).Update("status", status).Error; err != nil {
		return wrapDBError(err, "批量更新用户状态")
	}
	return nil
}

// UpdateUserIsAdminByUuids 批量设置用户管理员权限
// uuids: 用户 UUID 列表
// isAdmin: 管理员标志（0=普通用户, 1=管理员）
// 返回: 操作错误
func (r *userRepository) UpdateUserIsAdminByUuids(uuids []string, isAdmin int8) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Model(&model.UserInfo{}).Where("uuid IN ?", uuids).Update("is_admin", isAdmin).Error; err != nil {
		return wrapDBError(err, "批量更新用户管理员状态")
	}
	return nil
}

// SoftDeleteUserByUuids 批量软删除用户
// GORM 软删除：设置 deleted_at 字段而非真正删除
// uuids: 要删除的用户 UUID 列表
// 返回: 操作错误
func (r *userRepository) SoftDeleteUserByUuids(uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	// Delete: GORM 模型有 gorm.Model 时自动进行软删除
	if err := r.db.Where("uuid IN ?", uuids).Delete(&model.UserInfo{}).Error; err != nil {
		return wrapDBError(err, "批量删除用户")
	}
	return nil
}
