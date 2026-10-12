# 0005. 采用纯 gRPC 单协议架构与 Envoy grpc_json_transcoder 边缘网关

- **状态**: Accepted
- **日期**: 2026-10-10

## 上下文
在微服务落地过程中，如果微服务进程内同时开放 gRPC 与 HTTP 双协议，每个微服务需要管理两个端口，并在进程内托管双 Server 生命周期及自定义 JSON 响应包装器，增加了微服务端口分配的心智负担和代码复杂度。用户明确要求：“纯 gRPC 单协议，微服务只开一个端口”。

## 决策
1. **微服务全量纯 gRPC 单协议化**：
   - 所有业务微服务（`user_service`, `apply_service`, `friendship_service`, `group_service`, `message_service`）彻底剔除 HTTP Server，每个服务**仅监听 1 个 gRPC 端口**。
   - 彻底删除 `khttp.NewServer` 及 `customResponseEncoder` / `customErrorEncoder` 等胶水代码，微服务代码回归最纯粹的领域 gRPC 实现。
2. **边缘网关接入 Envoy (grpc_json_transcoder)**：
   - 借助 `protoc --descriptor_set_out=configs/proto.pb --include_imports` 一键生成全服务的二进制 Protobuf 描述符文件。
   - 采用官方现成成熟轮子 **Envoy**（镜像 `envoyproxy/envoy:v1.30.1`），挂载 `proto.pb` 并启用原生 `envoy.filters.http.grpc_json_transcoder`。
   - Envoy 直接读取 `.proto` 中的 `google.api.http` 注解，全自动把客户端的 HTTP/JSON 请求（GET/POST/PUT/DELETE 等）动态序列化为后端的 gRPC 二进制流并路由至对应服务。
3. **长连接专职解耦**：
   - Envoy 通过 `upgrade_configs: [{upgrade_type: websocket}]` 将 `/ws*` 长连接流量直通专职长连接服务（`ConnService`），保障长连接接入与实时消息下推。
4. **分层治理整合 (Defense in Depth)**：
   - **Envoy（门神）**：专职负责外部 HTTP 转码、WebSocket 升级、CORS 跨域、全局 15s 大超时兜底、Admin 运维面板 (:9901) 与结构化 JSON 访问日志。
   - **Kratos（内管）**：微服务全量装配官方核心中间件——Panic 崩溃防护 (`recovery.Recovery`)、OpenTelemetry 分布式追踪 (`tracing.Server`)、跨进程元数据透传 (`metadata.Server`) 与 2s 细粒度 RPC 超时控制。

## 结果
- 实现了真正的云原生最佳实践：**“外部统一标准 HTTP/JSON RESTful，内部 100% 纯 gRPC 单协议”**。
- 微服务端口占用大幅精简，零 HTTP 胶水代码，消除了双协议维护的复杂性。
- Envoy 与 Kratos 各司其职，形成了高效、清晰、无冗余的纵深防御体系。
