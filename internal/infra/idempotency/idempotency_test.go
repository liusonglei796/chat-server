package idempotency

import (
	"context"
	"errors"
	"testing"

	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/interceptor"
)

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantRetryable bool
		wantCode      int
	}{
		{
			name:          "nil error",
			err:           nil,
			wantRetryable: true,
			wantCode:      0,
		},
		{
			name:          "invalid param 4xx",
			err:           errorx.New(errorx.CodeInvalidParam, "参数错误"),
			wantRetryable: false,
			wantCode:      errorx.CodeInvalidParam,
		},
		{
			name:          "unauthorized 401",
			err:           errorx.New(errorx.CodeUnauthorized, "未登录"),
			wantRetryable: false,
			wantCode:      errorx.CodeUnauthorized,
		},
		{
			name:          "not found 404",
			err:           errorx.New(errorx.CodeNotFound, "用户不存在"),
			wantRetryable: false,
			wantCode:      errorx.CodeNotFound,
		},
		{
			name:          "server busy 503",
			err:           errorx.ErrServerBusy,
			wantRetryable: true,
			wantCode:      errorx.CodeServerBusy,
		},
		{
			name:          "conflict lock 409/retryable",
			err:           errorx.New(errorx.CodeConflict, "正在处理中"),
			wantRetryable: true,
			wantCode:      errorx.CodeConflict,
		},
		{
			name:          "db error 500",
			err:           errorx.New(errorx.CodeDBError, "数据库连接失败"),
			wantRetryable: true,
			wantCode:      errorx.CodeDBError,
		},
		{
			name:          "context deadline exceeded",
			err:           context.DeadlineExceeded,
			wantRetryable: true,
			wantCode:      errorx.CodeTimeout,
		},
		{
			name:          "raw unknown error",
			err:           errors.New("unknown error"),
			wantRetryable: false,
			wantCode:      errorx.CodeServerBusy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryable, code, _ := isRetryable(tt.err)
			if retryable != tt.wantRetryable {
				t.Errorf("isRetryable(%v) retryable = %v, want %v", tt.name, retryable, tt.wantRetryable)
			}
			if code != tt.wantCode {
				t.Errorf("isRetryable(%v) code = %v, want %v", tt.name, code, tt.wantCode)
			}
		})
	}
}

func TestRecordStatusConstants(t *testing.T) {
	if StatusProcessing != "PROCESSING" {
		t.Errorf("expected PROCESSING, got %s", StatusProcessing)
	}
	if StatusSuccess != "SUCCESS" {
		t.Errorf("expected SUCCESS, got %s", StatusSuccess)
	}
	if StatusFail != "FAIL" {
		t.Errorf("expected FAIL, got %s", StatusFail)
	}
}

func TestContextKeyUnification(t *testing.T) {
	ctx := context.Background()
	key := "test-idempotency-key-123"

	// 验证 ContextWithKey 注入后能被 interceptor 读取
	ctxWithKey := ContextWithKey(ctx, key)
	if got := interceptor.GetIdempotencyKeyFromContext(ctxWithKey); got != key {
		t.Errorf("interceptor.GetIdempotencyKeyFromContext() = %v, want %v", got, key)
	}
	if got := KeyFromContext(ctxWithKey); got != key {
		t.Errorf("KeyFromContext() = %v, want %v", got, key)
	}

	// 验证 interceptor.ContextWithIdempotencyKey 注入后能被 KeyFromContext 读取
	ctxWithInter := interceptor.ContextWithIdempotencyKey(ctx, key)
	if got := KeyFromContext(ctxWithInter); got != key {
		t.Errorf("KeyFromContext() from interceptor context = %v, want %v", got, key)
	}
}
