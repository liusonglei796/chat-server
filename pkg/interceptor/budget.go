package interceptor

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryBudgetInterceptor 检查上下文 Deadline 剩余超时时间预算的拦截器
// 在发起初次调用或重试前，评估剩余超时时间是否满足最小预算（minBudget）。
// 若剩余时间不足以支撑网络通信与服务处理，提前主动返回 codes.DeadlineExceeded 终止重试，避免无效请求堆积压垮下游服务。
func RetryBudgetInterceptor(minBudget time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining < minBudget {
				zap.L().Warn("gRPC 剩余时间预算不足，主动终止调用与重试",
					zap.String("method", method),
					zap.Duration("remaining", remaining),
					zap.Duration("minBudget", minBudget),
				)
				return status.Errorf(codes.DeadlineExceeded,
					"context deadline remaining budget (%v) is less than required min budget (%v)",
					remaining, minBudget,
				)
			}
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
