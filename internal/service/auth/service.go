// Package auth 提供认证相关的业务逻辑
// 处理 Token 验证、刷新等功能
package auth

import (
	"context"

	"kama_chat_server/internal/service/mysqlinterface"
	redisinterface "kama_chat_server/internal/service/redisinterface"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/errorx"
)

// Service 认证服务实现
type Service struct {
	cache    redisinterface.CacheService   // 缓存服务（依赖倒置）
	userRepo mysqlinterface.UserRepository // 用户仓库（用于获取管理员状态）
}

// NewAuthService 创建认证服务实例
// cache: 缓存服务接口实例
// userRepo: 用户仓库接口实例
func NewAuthService(cache redisinterface.CacheService, userRepo mysqlinterface.UserRepository) *Service {
	return &Service{
		cache:    cache,
		userRepo: userRepo,
	}
}

// ValidateTokenID 验证用户的 Token ID 是否有效
// 用于实现单点登录互踢机制
// userID: 用户ID
// tokenID: 需要验证的 Token ID
// 返回: 是否有效, 错误信息
func (s *Service) ValidateTokenID(ctx context.Context, userID, tokenID string) (bool, error) {
	redisKey := constants.CacheKeyUserToken + userID
	validTokenID, err := s.cache.Get(ctx, redisKey)
	if err != nil {
		return false, err
	}
	if validTokenID == "" {
		return false, nil
	}
	return tokenID == validTokenID, nil
}

// GetUserIsAdmin 获取用户是否为管理员
// 用于 Token 刷新时获取最新的管理员状态
func (s *Service) GetUserIsAdmin(ctx context.Context, userID string) (bool, error) {
	user, err := s.userRepo.FindByUuid(ctx, userID)
	if err != nil {
		if errorx.IsNotFound(err) {
			return false, errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		return false, err
	}
	return user.IsAdmin == 1, nil
}
