package repository

import (
	"kama_chat_server/internal/model"

	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建用户 Repository
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

// FindByUuid 按 UUID 查找用户
func (r *userRepository) FindByUuid(uuid string) (*model.UserInfo, error) {
	var user model.UserInfo
	if err := r.db.First(&user, "uuid = ?", uuid).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询用户 uuid=%s", uuid)
	}
	return &user, nil
}

// FindByTelephone 按电话查找用户
func (r *userRepository) FindByTelephone(telephone string) (*model.UserInfo, error) {
	var user model.UserInfo
	if err := r.db.First(&user, "telephone = ?", telephone).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询用户 telephone=%s", telephone)
	}
	return &user, nil
}

// FindAllExcept 查找除指定用户外的所有用户
func (r *userRepository) FindAllExcept(excludeUuid string) ([]model.UserInfo, error) {
	var users []model.UserInfo
	if err := r.db.Unscoped().Where("uuid != ?", excludeUuid).Find(&users).Error; err != nil {
		return nil, wrapDBError(err, "查询用户列表")
	}
	return users, nil
}

// FindByUuids 按 UUID 列表查找用户
func (r *userRepository) FindByUuids(uuids []string) ([]model.UserInfo, error) {
	var users []model.UserInfo
	if err := r.db.Where("uuid IN ?", uuids).Find(&users).Error; err != nil {
		return nil, wrapDBError(err, "批量查询用户")
	}
	return users, nil
}

// Create 创建用户
func (r *userRepository) Create(user *model.UserInfo) error {
	if err := r.db.Create(user).Error; err != nil {
		return wrapDBError(err, "创建用户")
	}
	return nil
}

// UpdateUserInfo 更新用户信息
func (r *userRepository) UpdateUserInfo(user *model.UserInfo) error {
	if err := r.db.Save(user).Error; err != nil {
		return wrapDBError(err, "更新用户信息")
	}
	return nil
}

// UpdateUserStatusByUuids 批量更新用户状态
func (r *userRepository) UpdateUserStatusByUuids(uuids []string, status int8) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Model(&model.UserInfo{}).Where("uuid IN ?", uuids).Update("status", status).Error; err != nil {
		return wrapDBError(err, "批量更新用户状态")
	}
	return nil
}

// UpdateUserIsAdminByUuids 更新用户是否为管理员
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
func (r *userRepository) SoftDeleteUserByUuids(uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Where("uuid IN ?", uuids).Delete(&model.UserInfo{}).Error; err != nil {
		return wrapDBError(err, "批量删除用户")
	}
	return nil
}
