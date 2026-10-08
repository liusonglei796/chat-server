package grpc_client_test

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	gatewaypb "kama_chat_server/api/gen/gateway"
	"kama_chat_server/pkg/interceptor"
)

// mockGatewayServiceForBreaker 用于测试熔断器的 Mock gRPC 服务
type mockGatewayServiceForBreaker struct {
	gatewaypb.UnimplementedGatewayServiceServer

	pushCallCount      int32
	batchPushCallCount int32

	pushErrHandler func(count int) error
}

func (s *mockGatewayServiceForBreaker) PushMessage(ctx context.Context, req *gatewaypb.PushMessageRequest) (*gatewaypb.PushMessageResponse, error) {
	cnt := int(atomic.AddInt32(&s.pushCallCount, 1))
	if s.pushErrHandler != nil {
		if err := s.pushErrHandler(cnt); err != nil {
			return nil, err
		}
	}
	return &gatewaypb.PushMessageResponse{Success: true}, nil
}

func (s *mockGatewayServiceForBreaker) BatchPushMessage(ctx context.Context, req *gatewaypb.BatchPushMessageRequest) (*gatewaypb.BatchPushMessageResponse, error) {
	atomic.AddInt32(&s.batchPushCallCount, 1)
	return &gatewaypb.BatchPushMessageResponse{}, nil
}

// startMockBreakerServer 启动本地轻量测试 gRPC 服务
func startMockBreakerServer(t *testing.T, svc *mockGatewayServiceForBreaker) (string, func()) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)

	s := grpc.NewServer()
	gatewaypb.RegisterGatewayServiceServer(s, svc)

	go func() {
		_ = s.Serve(lis)
	}()

	cleanup := func() {
		s.Stop()
		_ = lis.Close()
	}
	return lis.Addr().String(), cleanup
}

// TestCircuitBreaker_MethodLevelIsolation 验证核心准则：熔断器是方法级（Method-level）隔离
// MethodA (PushMessage) 发生系统崩溃并熔断后，MethodB (BatchPushMessage) 依然 100% 正常调用，互不株连！
func TestCircuitBreaker_MethodLevelIsolation(t *testing.T) {
	mockSvc := &mockGatewayServiceForBreaker{
		pushErrHandler: func(count int) error {
			// 始终返回 Unavailable 系统级错误
			return status.Errorf(codes.Unavailable, "mock system unavailable")
		},
	}

	addr, cleanup := startMockBreakerServer(t, mockSvc)
	defer cleanup()

	// 采用方法级熔断拦截器（阈值设置为 5 次连续失败即熔断）
	breakerInterceptor := interceptor.CircuitBreakerInterceptor(
		interceptor.WithBreakerTimeout(500 * time.Millisecond),
	)

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(breakerInterceptor),
	)
	assert.NoError(t, err)
	defer conn.Close()

	client := gatewaypb.NewGatewayServiceClient(conn)
	ctx := context.Background()

	// 1. 连续调用 PushMessage 5 次，使其达到连续 5 次失败熔断阈值
	for i := 0; i < 5; i++ {
		_, err := client.PushMessage(ctx, &gatewaypb.PushMessageRequest{TargetUserId: "u1"})
		assert.Error(t, err)
	}

	// 2. 验证第 6 次调用 PushMessage 已被熔断器快速失败拦截（Client-side Fast Fail）
	_, err = client.PushMessage(ctx, &gatewaypb.PushMessageRequest{TargetUserId: "u1"})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "circuit breaker open for method"), "应命中 PushMessage 熔断")

	// 3. 关键验证：同一服务内的 BatchPushMessage 接口完全不受影响，依然健康调用并成功！
	batchResp, err := client.BatchPushMessage(ctx, &gatewaypb.BatchPushMessageRequest{TargetUserIds: []string{"u1"}})
	assert.NoError(t, err, "Method-level 熔断隔离：BatchPushMessage 绝不能被 PushMessage 的熔断株连！")
	assert.NotNil(t, batchResp)
	assert.Equal(t, int32(1), atomic.LoadInt32(&mockSvc.batchPushCallCount))
}

// TestCircuitBreaker_ExcludeBusinessErrors 验证核心准则：业务逻辑错误与 4xx 不计入熔断失败率
// 即使连续发生 15 次业务参数错误，熔断器也绝对不会进入 Open 状态
func TestCircuitBreaker_ExcludeBusinessErrors(t *testing.T) {
	mockSvc := &mockGatewayServiceForBreaker{
		pushErrHandler: func(count int) error {
			// 模拟业务逻辑报错（400 参数非法）
			return status.Errorf(codes.InvalidArgument, "invalid user id")
		},
	}

	addr, cleanup := startMockBreakerServer(t, mockSvc)
	defer cleanup()

	breakerInterceptor := interceptor.CircuitBreakerInterceptor()

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(breakerInterceptor),
	)
	assert.NoError(t, err)
	defer conn.Close()

	client := gatewaypb.NewGatewayServiceClient(conn)
	ctx := context.Background()

	// 连续发起 15 次业务报错调用
	for i := 0; i < 15; i++ {
		_, err := client.PushMessage(ctx, &gatewaypb.PushMessageRequest{TargetUserId: "bad_user"})
		assert.Error(t, err)
		st, ok := status.FromError(err)
		assert.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	}

	// 验证服务端确实收到了全部 15 次请求，且熔断器未被触发（第 16 次仍然正常穿透至服务端）
	assert.Equal(t, int32(15), atomic.LoadInt32(&mockSvc.pushCallCount))
	_, err = client.PushMessage(ctx, &gatewaypb.PushMessageRequest{TargetUserId: "bad_user"})
	assert.Error(t, err)
	assert.False(t, strings.Contains(err.Error(), "circuit breaker open"), "业务错误绝不应该使熔断器跳闸！")
}

// TestCircuitBreaker_SelfHealing 验证核心准则：半开（Half-Open）探测与自愈恢复
// 熔断后经过冷却期，探测请求成功后，熔断器自动重置回 Closed 状态
func TestCircuitBreaker_SelfHealing(t *testing.T) {
	failSwitch := int32(1) // 1=fail, 0=success

	mockSvc := &mockGatewayServiceForBreaker{
		pushErrHandler: func(count int) error {
			if atomic.LoadInt32(&failSwitch) == 1 {
				return status.Errorf(codes.Unavailable, "downstream crash")
			}
			return nil
		},
	}

	addr, cleanup := startMockBreakerServer(t, mockSvc)
	defer cleanup()

	// 配置较短的冷却期 80ms 用于快速测试
	breakerInterceptor := interceptor.CircuitBreakerInterceptor(
		interceptor.WithBreakerTimeout(80 * time.Millisecond),
	)

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(breakerInterceptor),
	)
	assert.NoError(t, err)
	defer conn.Close()

	client := gatewaypb.NewGatewayServiceClient(conn)
	ctx := context.Background()

	// 1. 触发连续 5 次失败，使熔断器开启 (Open)
	for i := 0; i < 5; i++ {
		_, _ = client.PushMessage(ctx, &gatewaypb.PushMessageRequest{TargetUserId: "u1"})
	}

	// 2. 确认已开启熔断
	_, err = client.PushMessage(ctx, &gatewaypb.PushMessageRequest{TargetUserId: "u1"})
	assert.True(t, strings.Contains(err.Error(), "circuit breaker open"))

	// 3. 模拟下游服务恢复
	atomic.StoreInt32(&failSwitch, 0)

	// 4. 等待冷却期超时 (100ms > 80ms)，熔断器进入 Half-Open 状态
	time.Sleep(100 * time.Millisecond)

	// 5. 发送半开探测请求，应恢复成功！
	resp, err := client.PushMessage(ctx, &gatewaypb.PushMessageRequest{TargetUserId: "u1"})
	assert.NoError(t, err, "半开探测请求应成功通过并自愈")
	assert.True(t, resp.Success)

	// 6. 后续请求继续正常通行，熔断器已回到 Closed 状态
	resp2, err := client.PushMessage(ctx, &gatewaypb.PushMessageRequest{TargetUserId: "u1"})
	assert.NoError(t, err)
	assert.True(t, resp2.Success)
}
