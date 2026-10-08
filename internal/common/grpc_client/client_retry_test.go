package grpc_client

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	gatewaypb "kama_chat_server/api/gen/gateway"
	"kama_chat_server/pkg/interceptor"
)

type comprehensiveMockServer struct {
	gatewaypb.UnimplementedGatewayServiceServer
	callCount     int32
	receivedKeys  []string
	mu            sync.Mutex
	returnErrCode codes.Code
}

func (s *comprehensiveMockServer) PushMessage(ctx context.Context, req *gatewaypb.PushMessageRequest) (*gatewaypb.PushMessageResponse, error) {
	current := atomic.AddInt32(&s.callCount, 1)

	// 记录每次请求接收到的业务幂等键
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		keys := md.Get(string(interceptor.IdempotencyKey))
		s.mu.Lock()
		if len(keys) > 0 {
			s.receivedKeys = append(s.receivedKeys, keys[0])
		}
		s.mu.Unlock()
	}

	if s.returnErrCode == codes.InvalidArgument {
		return nil, status.Error(codes.InvalidArgument, "invalid argument error")
	}

	if current < 3 {
		// 模拟前 2 次因网络抖动报 codes.Unavailable
		return nil, status.Error(codes.Unavailable, "temporary connection reset")
	}
	return &gatewaypb.PushMessageResponse{Success: true}, nil
}

// 1. 验证重试共享同一个 Deadline、传递同一个业务幂等键、并在临时错误下恢复
func TestGrpcClient_RetryAndPassSameIdempotencyKey(t *testing.T) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)
	defer lis.Close()

	s := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.ServerAuthInterceptor()),
	)
	mockSvc := &comprehensiveMockServer{}
	gatewaypb.RegisterGatewayServiceServer(s, mockSvc)

	go func() { _ = s.Serve(lis) }()
	defer s.Stop()

	retryOpts := []retry.CallOption{
		retry.WithMax(3),
		retry.WithPerRetryTimeout(2 * time.Second),
		retry.WithBackoff(retry.BackoffExponentialWithJitter(10*time.Millisecond, 0.1)),
		retry.WithCodes(codes.Unavailable, codes.ResourceExhausted),
	}

	bufDialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			retry.UnaryClientInterceptor(retryOpts...),
			interceptor.RetryBudgetInterceptor(100*time.Millisecond),
			interceptor.ClientAuthInterceptor(),
		),
	)
	assert.NoError(t, err)
	defer conn.Close()

	client := gatewaypb.NewGatewayServiceClient(conn)

	// 客户端设置统一的 Parent Deadline (5秒) 并注入同一个业务幂等键
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const expectedIdempKey = "biz-idemp-key-99999"
	ctx = interceptor.ContextWithIdempotencyKey(ctx, expectedIdempKey)

	resp, err := client.PushMessage(ctx, &gatewaypb.PushMessageRequest{
		TargetUserId: "U_retry_test",
		MessageUuid:  "msg-123",
	})

	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, int32(3), atomic.LoadInt32(&mockSvc.callCount), "临时错误应重试直到第 3 次成功")

	// 验证所有重试请求接收到的业务幂等键完全相同
	mockSvc.mu.Lock()
	assert.Len(t, mockSvc.receivedKeys, 3, "每次重试都应携带业务幂等键")
	for i, k := range mockSvc.receivedKeys {
		assert.Equal(t, expectedIdempKey, k, "第 %d 次重试的业务幂等键必须一致", i+1)
	}
	mockSvc.mu.Unlock()
}

// 2. 验证超时预算不足时，主动终止调用与重试
func TestGrpcClient_AbortRetryWhenBudgetExhausted(t *testing.T) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)
	defer lis.Close()

	s := grpc.NewServer()
	mockSvc := &comprehensiveMockServer{}
	gatewaypb.RegisterGatewayServiceServer(s, mockSvc)

	go func() { _ = s.Serve(lis) }()
	defer s.Stop()

	retryOpts := []retry.CallOption{
		retry.WithMax(3),
		retry.WithBackoff(retry.BackoffLinear(50 * time.Millisecond)),
		retry.WithCodes(codes.Unavailable),
	}

	bufDialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			retry.UnaryClientInterceptor(retryOpts...),
			interceptor.RetryBudgetInterceptor(100*time.Millisecond), // 最小预算 100ms
		),
	)
	assert.NoError(t, err)
	defer conn.Close()

	client := gatewaypb.NewGatewayServiceClient(conn)

	// 设置一个仅剩 20ms 的超短 Deadline（远低于 100ms 最小预算）
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = client.PushMessage(ctx, &gatewaypb.PushMessageRequest{
		TargetUserId: "U_retry_test",
	})

	assert.Error(t, err)
	assert.Equal(t, codes.DeadlineExceeded, status.Code(err), "预算不足时应主动返回 DeadlineExceeded 终止请求")
	assert.Equal(t, int32(0), atomic.LoadInt32(&mockSvc.callCount), "预算不足时服务端不应收到任何调用")
}

// 3. 验证非临时错误（业务错误 InvalidArgument）绝不进行盲目重试
func TestGrpcClient_DoNotRetryBusinessErrors(t *testing.T) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)
	defer lis.Close()

	s := grpc.NewServer()
	mockSvc := &comprehensiveMockServer{
		returnErrCode: codes.InvalidArgument, // 业务参数错误
	}
	gatewaypb.RegisterGatewayServiceServer(s, mockSvc)

	go func() { _ = s.Serve(lis) }()
	defer s.Stop()

	retryOpts := []retry.CallOption{
		retry.WithMax(3),
		retry.WithCodes(codes.Unavailable, codes.ResourceExhausted),
	}

	bufDialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			retry.UnaryClientInterceptor(retryOpts...),
			interceptor.RetryBudgetInterceptor(100*time.Millisecond),
		),
	)
	assert.NoError(t, err)
	defer conn.Close()

	client := gatewaypb.NewGatewayServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = client.PushMessage(ctx, &gatewaypb.PushMessageRequest{
		TargetUserId: "U_retry_test",
	})

	assert.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, int32(1), atomic.LoadInt32(&mockSvc.callCount), "业务错误只能执行 1 次，绝对不可重试")
}
