// Package cache 提供缓存相关的工具函数
package cache

import (
	"context"
	"encoding/json"
	"time"

	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/pkg/errorx"

	"golang.org/x/sync/singleflight"
)

// Helper 缓存辅助工具
// 提供 Cache-Aside 模式的标准实现，包含空值缓存和 singleflight 保护
type Helper struct {
	cache myredis.CacheService
	sf    singleflight.Group
}

// NewHelper 创建缓存辅助工具
func NewHelper(cache myredis.CacheService) *Helper {
	return &Helper{
		cache: cache,
	}
}

// GetOrLoad 通用的 Cache-Aside 实现
// 1. 先查缓存
// 2. 缓存未命中时使用 singleflight 防止缓存击穿
// 3. 支持空值缓存防止缓存穿透
//
// 参数:
//   - ctx: 上下文
//   - key: 缓存键
//   - loader: 数据加载函数（查询数据库）
//   - ttl: 正常数据的缓存过期时间
//   - nullTTL: 空值标记的缓存过期时间（传 0 表示不缓存空值）
//   - result: 用于反序列化的结果指针
//
// 返回:
//   - error: 加载错误或反序列化错误
func (h *Helper) GetOrLoad(
	ctx context.Context,
	key string,
	loader func() (interface{}, error),
	ttl time.Duration,
	nullTTL time.Duration,
	result interface{},
) error {
	nullKey := key + ":null"

	// 1. 检查空值标记（防止缓存穿透）
	if nullTTL > 0 {
		if val, _ := h.cache.Get(ctx, nullKey); val == "1" {
			return errorx.New(errorx.CodeNotFound, "data not found (cached null)")
		}
	}

	// 2. 查询缓存
	cachedValue, err := h.cache.Get(ctx, key)
	if err == nil && cachedValue != "" {
		// 缓存命中，反序列化
		if err := json.Unmarshal([]byte(cachedValue), result); err == nil {
			return nil
		}
		// 反序列化失败，降级查库
	}

	// 3. 使用 singleflight 防止缓存击穿
	val, sfErr, _ := h.sf.Do(key, func() (interface{}, error) {
		// 再次检查缓存（double-check）
		if cached, _ := h.cache.Get(ctx, key); cached != "" {
			return cached, nil
		}

		// 调用 loader 查询数据库
		data, err := loader()
		if err != nil {
			if errorx.IsNotFound(err) && nullTTL > 0 {
				// 缓存空值标记
				_ = h.cache.Set(ctx, nullKey, "1", RandomizedTTL(nullTTL))
			}
			return nil, err
		}

		// 序列化并回写缓存
		jsonData, err := json.Marshal(data)
		if err != nil {
			return data, nil // 序列化失败不影响返回
		}
		_ = h.cache.Set(ctx, key, string(jsonData), RandomizedTTL(ttl))

		return data, nil
	})

	if sfErr != nil {
		return sfErr
	}

	// 处理 singleflight 返回值
	switch v := val.(type) {
	case string:
		// 从缓存读取的字符串
		return json.Unmarshal([]byte(v), result)
	default:
		// 从 loader 返回的对象，直接赋值
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
