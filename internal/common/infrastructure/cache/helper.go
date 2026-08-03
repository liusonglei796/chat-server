// Package cache 提供缓存相关的工具函数
package cache

import (
	"context"
	"encoding/json"
	"time"

	"kama_chat_server/internal/common/domain/repository"
	"kama_chat_server/internal/common/infrastructure/metrics"
	"kama_chat_server/pkg/errorx"

	"golang.org/x/sync/singleflight"
)

const (
	// DefaultLoaderTimeout loader 默认超时时间
	// 防止 DB 死锁或慢查询导致 singleflight 内所有等待者无限阻塞
	DefaultLoaderTimeout = 3 * time.Second
)

// Helper 缓存辅助工具
// 提供 Cache-Aside 模式的标准实现，包含空值缓存和 singleflight 保护
type Helper struct {
	cache         repository.CacheService
	sf            singleflight.Group
	loaderTimeout time.Duration
}

// HelperOption Helper 配置选项
type HelperOption func(*Helper)

// WithLoaderTimeout 设置 loader 超时时间
func WithLoaderTimeout(d time.Duration) HelperOption {
	return func(h *Helper) {
		h.loaderTimeout = d
	}
}

// NewHelper 创建缓存辅助工具
func NewHelper(cache repository.CacheService, opts ...HelperOption) *Helper {
	h := &Helper{
		cache:         cache,
		loaderTimeout: DefaultLoaderTimeout,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// GetOrLoad 通用的 Cache-Aside 实现
// 1. 先查缓存
// 2. 缓存未命中时使用 singleflight 防止缓存击穿
// 3. 支持空值缓存防止缓存穿透
// 4. loader 带超时控制，防止 DB 死锁导致所有请求阻塞
//
// 参数说明：
//   - ctx: 上下文，用于取消和超时控制
//   - key: 缓存键
//   - loader: 数据加载函数，接收带超时的 context，应将其传递给 DB 查询
//   - ttl: 缓存有效期
//   - nullTTL: 空值缓存有效期（防止缓存穿透），传 0 表示不缓存空值
//   - result: 用于接收结果的指针
func (h *Helper) GetOrLoad(
	ctx context.Context,
	key string,
	loader func(ctx context.Context) (interface{}, error),
	ttl time.Duration,
	nullTTL time.Duration,
	result interface{},
) error {
	nullKey := key + ":null"

	// 1. 检查空值缓存（防止缓存穿透）
	if nullTTL > 0 {
		if nullVal, _ := h.cache.Get(ctx, nullKey); nullVal == "1" {
			return errorx.New(errorx.CodeNotFound, "data not found (cached null)")
		}
	}

	// 2. 检查正常缓存
	cachedValue, err := h.cache.Get(ctx, key)
	if err == nil && cachedValue != "" {
		metrics.CacheHits.Inc()
		if err := json.Unmarshal([]byte(cachedValue), result); err == nil {
			return nil
		}
	}

	// 缓存未命中，记录指标
	metrics.CacheMisses.Inc()

	// 3. 创建带超时的 context，传递给 loader
	loaderCtx, cancel := context.WithTimeout(ctx, h.loaderTimeout)
	defer cancel()

	// 4. Singleflight 防止缓存击穿（使用 DoChan + select 实现超时退出）
	ch := h.sf.DoChan(key, func() (interface{}, error) {
		// 二次检查缓存（可能其他请求已经填充）
		if cached, _ := h.cache.Get(ctx, key); cached != "" {
			return cached, nil
		}

		// 调用 loader，传入带超时的 context
		data, err := loader(loaderCtx)
		if err != nil {
			// 区分超时和其他错误
			if loaderCtx.Err() == context.DeadlineExceeded {
				// 超时后主动 Forget，让后续请求可以重新发起
				h.sf.Forget(key)
				return nil, errorx.New(errorx.CodeTimeout, "数据加载超时，请稍后重试")
			}
			// 空值缓存
			if errorx.IsNotFound(err) && nullTTL > 0 {
				_ = h.cache.Set(ctx, nullKey, "1", RandomizedTTL(nullTTL))
			}
			return nil, err
		}

		// 缓存回写
		jsonData, err := json.Marshal(data)
		if err != nil {
			return data, nil
		}
		_ = h.cache.Set(ctx, key, string(jsonData), RandomizedTTL(ttl))

		return data, nil
	})

	// 等待结果或超时
	var sfResult interface{}
	select {
	case res := <-ch:
		if res.Err != nil {
			return res.Err
		}
		sfResult = res.Val
	case <-loaderCtx.Done():
		// 超时或取消，主动 Forget 释放阻塞的等待者
		h.sf.Forget(key)
		return errorx.New(errorx.CodeTimeout, "数据加载超时，请稍后重试")
	}

	// 5. 反序列化结果
	switch v := sfResult.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), result); err != nil {
			return errorx.Wrap(err, errorx.CodeCacheError, "缓存数据反序列化失败")
		}
		return nil
	default:
		jsonData, err := json.Marshal(v)
		if err != nil {
			return errorx.Wrap(err, errorx.CodeCacheError, "缓存数据序列化失败")
		}
		if err := json.Unmarshal(jsonData, result); err != nil {
			return errorx.Wrap(err, errorx.CodeCacheError, "缓存数据反序列化失败")
		}
		return nil
	}
}

// InvalidateWithNull 删除缓存并清除空值标记
func (h *Helper) InvalidateWithNull(ctx context.Context, key string) error {
	nullKey := key + ":null"
	if err := h.cache.Delete(ctx, key); err != nil {
		return errorx.Wrap(err, errorx.CodeCacheError, "删除缓存失败")
	}
	if err := h.cache.Delete(ctx, nullKey); err != nil {
		return errorx.Wrap(err, errorx.CodeCacheError, "删除空值缓存标记失败")
	}
	return nil
}
