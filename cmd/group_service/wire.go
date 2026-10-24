//go:build wireinject
// +build wireinject

package main

import (
	"fmt"
	"time"

	etcdregistry "github.com/go-kratos/kratos/contrib/registry/etcd/v2"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/middleware/metadata"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/registry"
	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/google/wire"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"

	grouppb "kama_chat_server/api/gen/group"
	"kama_chat_server/internal/group"
	"kama_chat_server/internal/infra"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/pkg/discovery"
)

func provideGRPCServer(groupServer *group.GrpcServer) *kgrpc.Server {
	grpcPort := 50055
	grpcAddr := fmt.Sprintf("0.0.0.0:%d", grpcPort)
	grpcSrv := kgrpc.NewServer(
		kgrpc.Address(grpcAddr),
		kgrpc.Timeout(2*time.Second),
		kgrpc.Middleware(
			recovery.Recovery(),
			tracing.Server(),
			metadata.Server(),
		),
	)
	grouppb.RegisterGroupServiceServer(grpcSrv, groupServer)
	return grpcSrv
}

func provideRegistrar() (registry.Registrar, func(), error) {
	serviceName := "group_service"
	grpcPort := 50055
	etcdEndpoints := []string{"etcd:2379", "127.0.0.1:2379"}
	etcdCli, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect etcd: %w", err)
	}

	kratosRegistrar := etcdregistry.New(etcdCli)

	legacyRegister, err := discovery.NewRegister(etcdEndpoints, discovery.ServerInfo{
		Name:   serviceName,
		Addr:   fmt.Sprintf("%s:%d", serviceName, grpcPort),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Warn("failed to register legacy etcd endpoints", zap.Error(err))
	}

	cleanup := func() {
		if legacyRegister != nil {
			legacyRegister.Stop()
		}
		_ = etcdCli.Close()
	}

	return kratosRegistrar, cleanup, nil
}

func newApp(srv *kgrpc.Server, r registry.Registrar, _ *group.DomainEventConsumer) *kratos.App {
	// 初始化内部微服务调用 gRPC 客户端
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	return kratos.New(
		kratos.Name("group_service"),
		kratos.Version("v1.0.0"),
		kratos.Server(srv),
		kratos.Registrar(r),
	)
}

// wireApp 依赖注入入口，由 Google Wire 编译生成 wire_gen.go
func wireApp(conf *config.Config) (*kratos.App, func(), error) {
	panic(wire.Build(
		infra.ProvideCache,
		group.ProviderSet,
		provideGRPCServer,
		provideRegistrar,
		newApp,
	))
}
