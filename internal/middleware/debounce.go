package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"

	"kama_chat_server/internal/infra/debounce"
	"kama_chat_server/internal/infra/idempotency"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/interceptor"
)

const debounceMessage = "请求过于频繁，请稍后再试"

// RequestDebounce 提供全局或接口级的秒级防抖中间件（基于 Redis SETNX 1s）
func RequestDebounce(store debounce.Store) middleware.Middleware {
	if store == nil {
		panic("debounce store is nil")
	}

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			userID := trustedUserID(ctx)
			if userID == "" {
				return handler(ctx, req)
			}

			operation := ""
			if tr, ok := transport.FromServerContext(ctx); ok {
				operation = tr.Operation()
				// 将请求头中的 Idempotency-Key / X-Idempotency-Key 注入 Context
				if key := tr.RequestHeader().Get("Idempotency-Key"); key != "" {
					ctx = idempotency.ContextWithKey(ctx, key)
				} else if key := tr.RequestHeader().Get("X-Idempotency-Key"); key != "" {
					ctx = idempotency.ContextWithKey(ctx, key)
				}
			}
			if operation == "" {
				return handler(ctx, req)
			}

			payload, err := json.Marshal(req)
			if err != nil {
				return nil, fmt.Errorf("marshal debounce request: %w", err)
			}
			sum := sha256.Sum256(payload)
			key := "debounce:" + userID + ":" + operation + ":" + hex.EncodeToString(sum[:])

			allowed, err := store.SetNX(ctx, key, "1", time.Second)
			if err != nil {
				return nil, errorx.ErrServerBusy
			}
			if !allowed {
				return nil, errorx.New(errorx.CodeConflict, debounceMessage)
			}

			return handler(ctx, req)
		}
	}
}

func trustedUserID(ctx context.Context) string {
	if tr, ok := transport.FromServerContext(ctx); ok {
		if userID := tr.RequestHeader().Get("X-User-Id"); userID != "" {
			return userID
		}
	}
	if value := ctx.Value("user_id"); value != nil {
		if userID, ok := value.(string); ok {
			return userID
		}
	}
	return interceptor.GetUserIDFromContext(ctx)
}
