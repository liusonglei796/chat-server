// Package interceptor 提供 gRPC 通用拦截器
// 本文件实现生产级方法级（Method-level）熔断器拦截器
package interceptor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BreakerConfig 熔断器通用配置选项
type BreakerConfig struct {
	// MaxRequests 半开 (Half-Open) 状态下允许通过的最大探测请求数（默认 2）
	MaxRequests uint32
	// Interval 闭合 (Closed) 状态下统计周期的清空窗口（默认 30s）
	Interval time.Duration
	// Timeout 开启 (Open) 状态下熔断冷却时长，超时后进入半开状态（默认 10s）
	Timeout time.Duration
	// ReadyToTrip 触发熔断的自定义评估函数
	ReadyToTrip func(counts gobreaker.Counts) bool
	// IsSuccessful 判断 RPC 调用是否成功的函数（至关重要：用于剔除业务错误）
	IsSuccessful func(err error) bool
}

// BreakerOption 熔断器配置函数
type BreakerOption func(*BreakerConfig)

// WithBreakerTimeout 自定义熔断冷却时间
func WithBreakerTimeout(d time.Duration) BreakerOption {
	return func(c *BreakerConfig) {
		c.Timeout = d
	}
}

// WithBreakerMaxRequests 自定义半开探测请求并发数
func WithBreakerMaxRequests(n uint32) BreakerOption {
	return func(c *BreakerConfig) {
		c.MaxRequests = n
	}
}

// WithBreakerInterval 自定义统计计数刷新周期
func WithBreakerInterval(d time.Duration) BreakerOption {
	return func(c *BreakerConfig) {
		c.Interval = d
	}
}

// WithReadyToTrip 自定义触发熔断判定规则
func WithReadyToTrip(fn func(counts gobreaker.Counts) bool) BreakerOption {
	return func(c *BreakerConfig) {
		c.ReadyToTrip = fn
	}
}

// defaultIsSuccessful 默认成功判定：严格剔除客户端错误与确定性业务逻辑报错
func defaultIsSuccessful(err error) bool {
	if err == nil {
		return true
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	// 核心黄金法则：
	// 客户端 4xx 错误以及下游明确返回的业务逻辑校验失败，代表下游服务进程是健康且正常响应的！
	// 绝对不能计入熔断器失败统计，否则恶意用户输错密码或传递非法参数会导致核心接口被误熔断 (DoS)！
	switch st.Code() {
	case codes.OK,
		codes.InvalidArgument,    // 400 客户端参数错误
		codes.NotFound,           // 404 资源不存在
		codes.AlreadyExists,      // 409 资源已存在冲突
		codes.PermissionDenied,   // 403 无权限
		codes.Unauthenticated,    // 401 未认证 / 登录态失效
		codes.FailedPrecondition: // 412/400 业务前置条件不满足（如余额不足、状态不对）
		return true
	default:
		// 仅系统级不可用、网络断开、死锁、连接拒绝、严重超时计入熔断器失败
		// 例如：codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.Unknown
		return false
	}
}

// defaultReadyToTrip 默认熔断判定规则：连续 5 次系统级故障，或 10 次以上请求且系统故障率达到 50%
func defaultReadyToTrip(counts gobreaker.Counts) bool {
	if counts.ConsecutiveFailures >= 5 {
		return true
	}
	if counts.Requests >= 10 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.5 {
		return true
	}
	return false
}

// MethodBreakerManager 方法级熔断器管理器
// 为每个 FullMethod 独立维护一套专属的 CircuitBreaker 实例，实现最彻底的故障舱壁隔离
type MethodBreakerManager struct {
	breakers sync.Map // map[string]*gobreaker.CircuitBreaker
	cfg      BreakerConfig
}

// NewMethodBreakerManager 创建方法级熔断器管理器
func NewMethodBreakerManager(opts ...BreakerOption) *MethodBreakerManager {
	cfg := BreakerConfig{
		MaxRequests:  2,
		Interval:     30 * time.Second,
		Timeout:      10 * time.Second,
		ReadyToTrip:  defaultReadyToTrip,
		IsSuccessful: defaultIsSuccessful,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &MethodBreakerManager{
		cfg: cfg,
	}
}

// GetBreaker 获取或动态初始化指定 FullMethod 的专属熔断器
func (m *MethodBreakerManager) GetBreaker(fullMethod string) *gobreaker.CircuitBreaker {
	if val, ok := m.breakers.Load(fullMethod); ok {
		return val.(*gobreaker.CircuitBreaker)
	}

	st := gobreaker.Settings{
		Name:        fullMethod,
		MaxRequests: m.cfg.MaxRequests,
		Interval:    m.cfg.Interval,
		Timeout:     m.cfg.Timeout,
		ReadyToTrip: m.cfg.ReadyToTrip,
		IsSuccessful: m.cfg.IsSuccessful,
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			zap.L().Warn("gRPC 方法级熔断器状态变迁",
				zap.String("method", name),
				zap.String("from_state", from.String()),
				zap.String("to_state", to.String()),
			)
		},
	}

	cb := gobreaker.NewCircuitBreaker(st)
	actual, _ := m.breakers.LoadOrStore(fullMethod, cb)
	return actual.(*gobreaker.CircuitBreaker)
}

// CircuitBreakerInterceptor 方法级 gRPC 客户端熔断拦截器
// 放在 Unary 拦截器链的最外层：当某接口方法发生熔断时，立即在客户端直接快速失败，不再发起重试与网络调用
func CircuitBreakerInterceptor(opts ...BreakerOption) grpc.UnaryClientInterceptor {
	manager := NewMethodBreakerManager(opts...)

	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		cb := manager.GetBreaker(method)

		// 通过 gobreaker.Execute 执行受保护的 RPC 调用
		_, err := cb.Execute(func() (any, error) {
			invErr := invoker(ctx, method, req, reply, cc, callOpts...)
			return nil, invErr
		})

		if err != nil {
			// 熔断器处于 Open 状态或半开超过并发限制时快速失败
			if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
				return status.Errorf(
					codes.Unavailable,
					"circuit breaker open for method %s: downstream method is unhealthy, fast-failing in client",
					method,
				)
			}
			return err
		}

		return nil
	}
}
