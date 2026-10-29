//go:build wireinject
// +build wireinject

package main

import (
	"fmt"
	"net"

	"github.com/google/wire"
	"google.golang.org/grpc"

	gatewaypb "kama_chat_server/api/gen/gateway"
	"kama_chat_server/internal/gateway"
	"kama_chat_server/internal/infra"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/pkg/interceptor"
	otelinit "kama_chat_server/pkg/otel"
)

// GrpcServerWrapper 网关内部 gRPC 服务包装
type GrpcServerWrapper struct {
	Server   *grpc.Server
	Listener net.Listener
	Port     int
	Addr     string
}

// GatewayApp 聚合连接中枢所有的运行时服务器与生命周期
type GatewayApp struct {
	ChatServer *gateway.ChatServer
	WsServer   *gateway.WsServer
	GrpcWrap   *GrpcServerWrapper
}

func provideGrpcComponents(conf *config.Config, srvImpl *gateway.GatewayGrpcServer) (*GrpcServerWrapper, func(), error) {
	grpcPort := conf.MainConfig.GrpcPort
	if grpcPort == 0 {
		grpcPort = 8001
	}
	grpcAddr := conf.MainConfig.GrpcAddr
	if grpcAddr == "" {
		grpcAddr = fmt.Sprintf("127.0.0.1:%d", grpcPort)
	}

	grpcLis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", grpcPort))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen on gateway grpc port: %w", err)
	}

	gatewayGrpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptor.ServerAuthInterceptor(),
			interceptor.ServerDeadlineBudgetInterceptor(interceptor.DefaultServerMinBudget),
			otelinit.ServerTraceInterceptor(),
		),
	)
	gatewaypb.RegisterGatewayServiceServer(gatewayGrpcServer, srvImpl)

	cleanup := func() {
		gatewayGrpcServer.GracefulStop()
	}

	wrapper := &GrpcServerWrapper{
		Server:   gatewayGrpcServer,
		Listener: grpcLis,
		Port:     grpcPort,
		Addr:     grpcAddr,
	}
	return wrapper, cleanup, nil
}

func newGatewayApp(
	chatServer *gateway.ChatServer,
	wsServer *gateway.WsServer,
	grpcWrap *GrpcServerWrapper,
) *GatewayApp {
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	return &GatewayApp{
		ChatServer: chatServer,
		WsServer:   wsServer,
		GrpcWrap:   grpcWrap,
	}
}

// wireApp 依赖注入入口，由 Google Wire 编译生成 wire_gen.go
func wireApp(conf *config.Config) (*GatewayApp, func(), error) {
	panic(wire.Build(
		infra.ProvideCache,
		gateway.ProviderSet,
		provideGrpcComponents,
		newGatewayApp,
	))
}
