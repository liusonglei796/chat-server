//go:build wireinject
// +build wireinject

package main

import (
	"fmt"
	"os"
	"strings"
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

	authpb "kama_chat_server/api/gen/auth"
	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/infra"
	"kama_chat_server/internal/infra/config"
	"kama_chat_server/internal/infra/debounce"
	"kama_chat_server/internal/middleware"
	"kama_chat_server/internal/user"
	"kama_chat_server/pkg/discovery"
	"kama_chat_server/pkg/interceptor"
)

func provideGRPCServer(userServer *user.UserGrpcServer, authServer *user.AuthGrpcServer, debounceStore debounce.Store) *kgrpc.Server {
	grpcPort := 50051
	grpcAddr := fmt.Sprintf("0.0.0.0:%d", grpcPort)
	grpcSrv := kgrpc.NewServer(
		kgrpc.Address(grpcAddr),
		kgrpc.Timeout(15*time.Second),
		kgrpc.Middleware(
			recovery.Recovery(),
			tracing.Server(),
			metadata.Server(),
			interceptor.ServerDeadlineBudgetMiddleware(interceptor.DefaultServerMinBudget),
			middleware.RequestDebounce(debounceStore),
		),
	)
	userpb.RegisterUserServiceServer(grpcSrv, userServer)
	authpb.RegisterAuthServiceServer(grpcSrv, authServer)
	return grpcSrv
}

func provideRegistrar() (registry.Registrar, func(), error) {
	serviceName := "user_service"
	grpcPort := 50051
	etcdEndpoints := []string{"etcd:2379", "127.0.0.1:2379"}
	etcdCli, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect etcd: %w", err)
	}

	kratosRegistrar := etcdregistry.New(etcdCli)

	regHost := strings.ReplaceAll(serviceName, "_", "-")
	if h := os.Getenv("SERVICE_HOST"); h != "" {
		regHost = h
	}
	legacyRegister, err := discovery.NewRegister(etcdEndpoints, discovery.ServerInfo{
		Name:   serviceName,
		Addr:   fmt.Sprintf("%s:%d", regHost, grpcPort),
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

func newApp(srv *kgrpc.Server, r registry.Registrar) *kratos.App {
	return kratos.New(
		kratos.Name("user_service"),
		kratos.Version("v1.0.0"),
		kratos.Server(srv),
		kratos.Registrar(r),
	)
}

// wireApp 依赖注入入口，由 Google Wire 编译生成 wire_gen.go
func wireApp(conf *config.Config) (*kratos.App, func(), error) {
	panic(wire.Build(
		infra.ProvideCache,
		infra.ProvideDebounceStore,
		user.ProviderSet,
		provideGRPCServer,
		provideRegistrar,
		newApp,
	))
}
