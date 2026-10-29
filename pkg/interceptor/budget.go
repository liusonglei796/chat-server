package interceptor

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/middleware"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// DefaultServerMinBudget 服务端入口初筛最小预算（100ms）
	DefaultServerMinBudget = 100 * time.Millisecond
	// DefaultClientRPCRetryBudget 跨网 RPC 下游调用前最小预算（150ms）
	DefaultClientRPCRetryBudget = 150 * time.Millisecond
	// DefaultDBCriticalBudget 重度数据库事务 / 回源前最小预算（200ms）
	DefaultDBCriticalBudget = 200 * time.Millisecond
)

// CheckRemainingBudget 检查 context 中的 deadline 剩余时间是否充足。
// 若剩余时间不足 minBudget，则主动短路并返回 codes.DeadlineExceeded，避免无效执行与雪崩级联。
func CheckRemainingBudget(ctx context.Context, minBudget time.Duration) error {
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < minBudget {
			zap.L().Warn("context deadline remaining budget insufficient, aborting early",
				zap.Duration("remaining", remaining),
				zap.Duration("minBudget", minBudget),
			)
			return status.Errorf(codes.DeadlineExceeded,
				"context deadline remaining budget (%v) is less than required min budget (%v)",
				remaining, minBudget,
			)
		}
	}
	return nil
}

// ServerDeadlineBudgetMiddleware 适用于 Kratos 服务端的请求入口拦截中间件
func ServerDeadlineBudgetMiddleware(minBudget time.Duration) middleware.Middleware {
	if minBudget <= 0 {
		minBudget = DefaultServerMinBudget
	}
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			if err := CheckRemainingBudget(ctx, minBudget); err != nil {
				return nil, err
			}
			return handler(ctx, req)
		}
	}
}

// ServerDeadlineBudgetInterceptor 适用于原生 gRPC Server 的服务端拦截器
func ServerDeadlineBudgetInterceptor(minBudget time.Duration) grpc.UnaryServerInterceptor {
	if minBudget <= 0 {
		minBudget = DefaultServerMinBudget
	}
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := CheckRemainingBudget(ctx, minBudget); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// RetryBudgetInterceptor 检查 Deadline 剩余有效超时时间预算。
// 用于发起 gRPC 跨网调用前，确保剩余时间充足；不足则直接返回 codes.DeadlineExceeded 中止调用。
func RetryBudgetInterceptor(minBudget time.Duration) grpc.UnaryClientInterceptor {
	if minBudget <= 0 {
		minBudget = DefaultClientRPCRetryBudget
	}
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if err := CheckRemainingBudget(ctx, minBudget); err != nil {
			return err
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
