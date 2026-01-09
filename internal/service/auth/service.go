// Package auth 提供认证相关的业务逻辑
// 处理 Token 验证、刷新等功能
package auth

import (
	"context"

	myredis "kama_chat_server/internal/dao/redis"
)

// Service 认证服务实现
type Service struct {
	cache myredis.CacheService // 缓存服务（依赖倒置）
}

// NewAuthService 创建认证服务实例
// cache: 缓存服务接口实例
func NewAuthService(cache myredis.CacheService) *Service {
	return &Service{
		cache: cache,
	}
}

// ValidateTokenID 验证用户的 Token ID 是否有效
// 用于实现单点登录互踢机制
// userID: 用户ID
// tokenID: 需要验证的 Token ID
// 返回: 是否有效, 错误信息
func (s *Service) ValidateTokenID(userID, tokenID string) (bool, error) {
	redisKey := "user_token:" + userID
	validTokenID, err := s.cache.Get(context.Background(), redisKey)
	if err != nil {
		return false, err
	}
	if validTokenID == "" {
		return false, nil
	}
	return tokenID == validTokenID, nil
}
