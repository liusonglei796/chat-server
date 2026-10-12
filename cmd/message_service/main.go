package main

import (
	"context"
	"fmt"
	"log"
	"time"

	etcdregistry "github.com/go-kratos/kratos/contrib/registry/etcd/v2"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/middleware/metadata"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"

	messagepb "kama_chat_server/api/gen/message"
	"kama_chat_server/internal/apps/message/message"
	"kama_chat_server/internal/apps/message/session"
	"kama_chat_server/internal/common/config"
	mysqlimpl "kama_chat_server/internal/common/dao/mysql"
	myredis "kama_chat_server/internal/common/dao/redis"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/internal/common/infrastructure/kafka"
	"kama_chat_server/internal/common/infrastructure/logger"
	"kama_chat_server/pkg/discovery"
	otelinit "kama_chat_server/pkg/otel"
)

func main() {
	// Step 1.1: 加载应用主配置
	// 来源: configs/config.toml 与环境变量
	// 用于: Step 1.2 日志系统与 Step 1.3 链路追踪初始化
	conf := config.GetConfig()

	// Step 1.2: 初始化结构化日志
	// 来源: Step 1.1 的 LogConfig
	// 用于: 全局 zap 日志输出
	if err := logger.Init(&conf.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}

	// Step 1.3: 初始化 OpenTelemetry 链路追踪
	// 来源: Step 1.1 的 OtelConfig
	// 用于: 服务退出时安全刷新未导出的追踪 span
	var otelShutdown func(context.Context) error
	if conf.OtelConfig.Enabled {
		var err error
		otelShutdown, err = otelinit.InitTracer(context.Background(), conf.OtelConfig.Endpoint, "message_service")
		if err != nil {
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
		zap.L().Info("OpenTelemetry 初始化成功",
			zap.String("endpoint", conf.OtelConfig.Endpoint),
			zap.String("serviceName", "message_service"),
		)
	}

	// Step 2.1: 初始化持久化数据层连接
	// 来源: 消息与会话服务专用数据库配置 (chat_message)
	// 用于: Step 3.1 实例化 MessageService 与 SessionService
	stores := mysqlimpl.InitFor("message")

	// Step 2.2: 初始化 Redis 异步缓存服务
	// 来源: Step 1.1 的 RedisConfig
	// 用于: Step 3.1 领域层消息与会话缓存
	cacheService := myredis.Init()
	var cachePort store.AsyncCacheService = cacheService

	// Step 2.3: 初始化内部微服务 gRPC 客户端
	// 来源: etcd 注册中心地址
	// 用于: 跨服务查询关系、用户及群组信息
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	// Step 3.1: 初始化领域服务与 gRPC 服务实现
	// 来源: Step 2.1 数据存储与 Step 2.2 缓存接口
	// 用于: Step 4.1 纯 gRPC 协议服务挂载
	msgSvc := message.NewMessageService(stores.Message, stores.Session, cachePort)
	sessionSvc := session.NewSessionService(stores.Session, stores.Message, cachePort)
	messageServerImpl := message.NewGrpcServer(msgSvc, sessionSvc)

	// Step 3.2: 启动领域事件消费者 (Kafka 异步消费，消费 outbox 发布的事件)
	// 来源: stores.Session 与 cachePort
	// 用于: 维护本地 session 冗余字段
	if err := kafka.EnsureTopic(context.Background(), kafka.TopicDomainEvents); err != nil {
		zap.L().Fatal("failed to ensure domain_events topic", zap.Error(err))
	}
	eventHandler := message.NewSessionEventHandler(stores.Session, cachePort)
	eventConsumer := message.NewDomainEventConsumer(eventHandler)
	eventConsumer.Start()
	defer eventConsumer.Close()

	// Step 4.1: 构建 Kratos 纯 gRPC 传输服务 (装配 Recovery 防崩溃、Tracing 链路追踪、Metadata 元数据透传)
	// 来源: Step 3.1 的服务实现实例与端口配置
	// 用于: Step 5.1 注册进 Kratos 运行时生命周期
	grpcPort := 50056
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
	messagepb.RegisterMessageServiceServer(grpcSrv, messageServerImpl)

	// Step 4.2: 实例化 Etcd 官方客户端与 Kratos 服务注册中心
	// 来源: etcd 节点连接列表
	// 用于: Step 5.1 Kratos 服务自发现与下线广播
	etcdEndpoints := []string{"etcd:2379", "127.0.0.1:2379"}
	etcdCli, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		zap.L().Fatal("failed to connect etcd", zap.Error(err))
	}
	defer etcdCli.Close()
	kratosRegistrar := etcdregistry.New(etcdCli)

	// Step 4.3: 维持旧版服务发现兼容注册 (供尚未改造的遗留网关及 CDC 服务发现)
	// 来源: Step 4.1 的 gRPC 监听地址与 etcd 客户端
	// 用于: 保证平滑灰度迁移期间其他旧服务正常通过 gRPC 互调
	legacyRegister, err := discovery.NewRegister(etcdEndpoints, discovery.ServerInfo{
		Name:   "message_service",
		Addr:   fmt.Sprintf("message-service:%d", grpcPort),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Warn("failed to register legacy etcd endpoints", zap.Error(err))
	} else {
		defer legacyRegister.Stop()
	}

	// Step 5.1: 组装并启动 Kratos 应用 (纯 gRPC 单协议)
	// 来源: Step 4.1 纯 gRPC 服务及 Step 4.2 Kratos 注册器
	// 用于: 统一管理单一 gRPC 端口监听、健康自省与优雅停机
	app := kratos.New(
		kratos.Name("message_service"),
		kratos.Version("v1.0.0"),
		kratos.Server(
			grpcSrv,
		),
		kratos.Registrar(kratosRegistrar),
	)

	zap.L().Info("Message Service (Pure gRPC Single Protocol) starting...",
		zap.String("grpcAddr", grpcAddr),
	)

	// Step 5.2: 阻塞运行应用直到接收到终止信号
	// 来源: OS SIGINT / SIGTERM 操作系统退出信号
	// 用于: 触发应用反注册及 gRPC 服务优雅停止
	if err := app.Run(); err != nil {
		zap.L().Fatal("message service exited with error", zap.Error(err))
	}

	// Step 6.1: 优雅清理 Redis 异步任务池与监控追踪资源
	// 来源: Step 2.2 的 Redis 实例与 Step 1.3 的 otelShutdown 闭包
	// 用于: 保证已入队异步操作落盘，未导出 trace span 刷新至采集端
	if rc, ok := cacheService.(*myredis.RedisCache); ok {
		rc.Release()
	}
	if otelShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			zap.L().Error("failed to shutdown tracer", zap.Error(err))
		}
	}
	zap.L().Info("Message Service stopped successfully")
}
