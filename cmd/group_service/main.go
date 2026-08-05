// group_service 服务进程入口
// 职责：加载配置、初始化依赖（MySQL/Redis/Kafka/etcd/OTel）、组装 group 业务服务、
// 注册 gRPC 接口与 etcd 服务发现、启动事件消费与 Outbox 发布、处理优雅退出。
package main

import (
	"context"   // 创建带超时/取消的上下文（OTel 关闭与退出场景）
	"fmt"       // 格式化字符串（构造监听地址、etcd 注册地址）
	"log"       // 标准库日志（仅 logger 初始化失败时的兜底输出）
	"net"       // TCP 监听（gRPC 服务需要底层网络连接）
	"os"        // 信号通道（SIGINT/SIGTERM 优雅退出）
	"os/signal" // 捕获系统信号
	"syscall"   // 定义 SIGINT/SIGTERM 常量
	"time"      // OTel 关闭超时

	"go.uber.org/zap"                   // 结构化日志库（全项目统一日志）
	"google.golang.org/grpc"            // gRPC 服务端框架
	"google.golang.org/grpc/reflection" // gRPC reflection 协议（调试/grpcurl 动态查询接口用）

	grouppb "kama_chat_server/api/gen/group"               // 生成的 group proto 代码（接口定义 + 注册函数）
	"kama_chat_server/internal/apps/group"                 // group 业务服务（业务逻辑所在）
	"kama_chat_server/internal/common/config"              // 全局配置加载（读 configs/config.toml）
	mysqlimpl "kama_chat_server/internal/common/dao/mysql" // MySQL 仓库实现（NewStores 等）
	myredis "kama_chat_server/internal/common/dao/redis"   // Redis 缓存实现
	"kama_chat_server/internal/common/domain/store"        // 领域层接口（AsyncCacheService 等）
	"kama_chat_server/internal/common/grpc_client"         // 跨服务 gRPC 客户端（查 user 昵称/头像等）
	"kama_chat_server/internal/common/infrastructure/kafka"
	"kama_chat_server/internal/common/infrastructure/logger"        // zap logger 初始化
	outbox "kama_chat_server/internal/common/infrastructure/outbox" // Outbox 模式的事件发布器
	"kama_chat_server/pkg/discovery"                                // etcd 服务发现注册
	"kama_chat_server/pkg/interceptor"                              // gRPC 拦截器（JWT 鉴权）
	otelinit "kama_chat_server/pkg/otel"                            // OpenTelemetry 链路追踪初始化
)

func main() {
	// 加载全局配置（configs/config.toml），包含日志/Kafka/etcd/OTel 等配置项
	conf := config.GetConfig()

	// 初始化 zap 日志（环境 dev）；失败时 zap 尚未就绪，只能用标准库 log 兜底
	if err := logger.Init(&conf.LogConfig, "dev"); err != nil {
		log.Fatalf("init logger failed: %v", err)
	}

	// 声明 OTel 关闭函数变量——OTel 可能未启用，用它判断"是否需要关闭"
	var otelShutdown func(context.Context) error
	// 配置开启 OTel 时初始化链路追踪，服务名 group_service（监控面板区分服务）
	if conf.OtelConfig.Enabled {
		var err error
		otelShutdown, err = otelinit.InitTracer(context.Background(), conf.OtelConfig.Endpoint, "group_service")
		if err != nil {
			// Fatal：打印日志后直接退出（exit 1）
			zap.L().Fatal("OpenTelemetry 初始化失败", zap.Error(err))
		}
	}

	// 初始化 MySQL 连接并构造全部仓库；"group" 让连接指向 group 专属库（单服务单库架构）
	// 返回 *Stores 实现了所有服务的 UoW 子接口，可传给 group 服务
	stores := mysqlimpl.InitFor("group")
	// 初始化 Redis；显式声明为接口类型做依赖注入——业务层只依赖接口不依赖具体实现
	cacheService := myredis.Init()
	var cachePort store.AsyncCacheService = cacheService

	// 初始化跨服务 gRPC 客户端（经 etcd 解析对端地址）；须在启动前完成，否则 RPC 调用会 panic
	grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})

	// 创建 group 业务服务；stores 以 groupUoW 子接口传入——编译期保证只访问 Group/GroupMember 仓库
	groupSvc := group.NewGroupService(stores, cachePort)
	// 创建并启动 Kafka 领域事件消费者（消费 group 关心的事件，如群申请通过后真正加人进群）
	consumer := group.NewDomainEventConsumer(stores)
	consumer.Start()
	// 进程退出时关闭 reader
	defer consumer.Close()

	// 启动 Outbox 发布器：轮询 outbox 表把待发事件投递到 Kafka（本地事务+事件模式的后半段）
	outbox.NewPublisher(stores.Outbox, kafka.NewProducer(kafka.TopicDomainEvents)).Start()

	// group 服务监听端口 50055（约定：apply 50053 / friendship 50054 / group 50055）
	port := 50055
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		zap.L().Fatal("failed to listen", zap.Error(err))
	}

	// 创建 gRPC 服务器并挂两个一元拦截器（按序执行）：
	// 1. ServerAuthInterceptor：解析 JWT 校验调用方身份，用户 ID 注入 context
	// 2. ServerTraceInterceptor：提取/创建 trace 上下文，串联整条调用链
	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptor.ServerAuthInterceptor(),
			otelinit.ServerTraceInterceptor(),
		),
	)
	// 注册业务服务；NewGrpcServer 是协议适配层：把 proto 的 Request/Response 签名
	// 翻译成业务服务的 (ctx, string, DTO) → error 签名；内嵌 Unimplemented 使未实现 RPC 返回 Unimplemented
	grouppb.RegisterGroupServiceServer(s, group.NewGrpcServer(groupSvc))
	// 开启 reflection：允许 grpcurl 等工具动态查询服务接口（纯调试便利）
	reflection.Register(s)

	// 向 etcd 注册服务实例：服务名 group_service、地址 group_service:50055（docker 网络内互访）、
	// 权重 1（负载均衡）、租约 10 秒（需定期续约）
	register, err := discovery.NewRegister([]string{"etcd:2379", "127.0.0.1:2379"}, discovery.ServerInfo{
		Name:   "group_service",
		Addr:   fmt.Sprintf("group_service:%d", port),
		Weight: 1,
	}, 10)
	if err != nil {
		zap.L().Fatal("failed to register to etcd", zap.Error(err))
	}
	// 退出时向 etcd 反注册
	defer register.Stop()

	// goroutine 中启动 gRPC 服务（Serve 阻塞，放 goroutine）；主 goroutine 继续等待退出信号
	go func() {
		zap.L().Info("group_service started", zap.String("addr", addr))
		if err := s.Serve(lis); err != nil {
			zap.L().Fatal("failed to serve", zap.Error(err))
		}
	}()

	// 注册 SIGINT（Ctrl+C）/SIGTERM（docker stop），阻塞主 goroutine 等待信号——优雅退出的等待点
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zap.L().Info("Shutting down group_service...")
	// gRPC 优雅停止：不再接受新请求，等待正在处理的请求完成后关闭连接
	s.GracefulStop()

	// 关闭 Redis 连接池；cacheService 静态类型是接口，断言回具体 *RedisCache 确认有 Release
	if rc, ok := cacheService.(*myredis.RedisCache); ok {
		rc.Release()
	}

	// 若 OTel 已启用，5 秒超时内冲刷未上报的 trace span 到 collector，防止退出丢追踪数据
	if otelShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			zap.L().Error("failed to shutdown tracer", zap.Error(err))
		}
	}
	// 记录退出日志，main 返回，进程正常结束（exit 0）
	zap.L().Info("group_service stopped")
}
