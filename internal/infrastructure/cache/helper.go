// Package cache 提供缓存相关的工具函数
package cache

import (
	"context"
	"encoding/json"
	"time"

	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/pkg/errorx"

	"golang.org/x/sync/singleflight"
)

// Helper 缓存辅助工具
// 提供 Cache-Aside 模式的标准实现，包含空值缓存和 singleflight 保护
type Helper struct {
	cache repository.CacheService
	sf    singleflight.Group
}

// NewHelper 创建缓存辅助工具
func NewHelper(cache repository.CacheService) *Helper {
	return &Helper{
		cache: cache,
	}
}

// GetOrLoad 通用的 Cache-Aside 实现
// 1. 先查缓存
// 2. 缓存未命中时使用 singleflight 防止缓存击穿
// 3. 支持空值缓存防止缓存穿透
func (h *Helper) GetOrLoad(
	ctx context.Context,
	key string,
	loader func() (interface{}, error),
	ttl time.Duration,
	nullTTL time.Duration,
	result interface{},
) error {
	nullKey := key + ":null"

	if nullTTL > 0 {
		if nullVal, _ := h.cache.Get(ctx, nullKey); nullVal == "1" {
			return errorx.New(errorx.CodeNotFound, "data not found (cached null)")
		}
	}

	cachedValue, err := h.cache.Get(ctx, key)
	if err == nil && cachedValue != "" {
		if err := json.Unmarshal([]byte(cachedValue), result); err == nil {
			return nil
		}
	}

	sfResult, sfErr, _ := h.sf.Do(key, func() (interface{}, error) {
		if cached, _ := h.cache.Get(ctx, key); cached != "" {
			return cached, nil
		}

		data, err := loader()
		if err != nil {
			if errorx.IsNotFound(err) && nullTTL > 0 {
				_ = h.cache.Set(ctx, nullKey, "1", RandomizedTTL(nullTTL))
			}
			return nil, err
		}

		jsonData, err := json.Marshal(data)
		if err != nil {
			return data, nil
		}
		_ = h.cache.Set(ctx, key, string(jsonData), RandomizedTTL(ttl))

		return data, nil
	})

	if sfErr != nil {
		return sfErr
	}

	switch v := sfResult.(type) {
	case string:
		return json.Unmarshal([]byte(v), result)
	default:
		jsonData, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return json.Unmarshal(jsonData, result)
	}
}

// InvalidateWithNull 删除缓存并清除空值标记
func (h *Helper) InvalidateWithNull(ctx context.Context, key string) error {
	nullKey := key + ":null"
	if err := h.cache.Delete(ctx, key); err != nil {
		return err
	}
	return h.cache.Delete(ctx, nullKey)
}
