# 0012. 统一重构连接服务包路径与文件命名为 Connector / UserConn

- **状态**: Accepted
- **日期**: 2026-11-07

## 上下文
在 ADR 0010 中，团队已正式将 WebSocket 维持服务从原先容易与 Envoy 边缘网关混淆的“网关（Gateway）”更名为“连接服务（ConnectionService / Connector）”。

然而在工程实际落地中，代码仓内依然残留了大量的命名遗留与违背：
1. 核心单连接抽象文件依然沿用历史旧名 `internal/gateway/ws_gateway.go`，直接违背了 `GLOSSARY.md` 中将 `WsGateway` 列为 `_Avoid_` 的领域约束；
2. 内部包目录仍为 `internal/gateway`（包名 `package gateway`），运行入口仍为 `cmd/gateway`；
3. `docker-compose.yml` 与 `Dockerfile` 的构建参数仍指向 `SERVICE_PATH=./cmd/gateway`。

为了彻底统一领域模型与代码实现的语义一致性，消除一切术语歧义，决定对连接服务的文件、包路径与工程入口执行全面重命名。

## 决策

1. **核心连接抽象文件重命名（Single Responsibility Alignment）**：
   - 将 `ws_gateway.go` 重命名为 `user_conn.go`，对应单元测试重命名为 `user_conn_test.go`。
   - 文件职责严格收敛为承载单个在线用户连接抽象 `UserConn`、底层 `writePump` / `readPump` 循环、极速心跳判定 `isPingPayload` 与单次反序列化帧 `WsFrame`。

2. **内部模块与入口重命名（Package & Cmd Migration）**：
   - 将包目录 `internal/gateway` 整体迁移为 `internal/connector`，包声明全量更新为 `package connector`。
   - 将启动入口 `cmd/gateway` 整体迁移为 `cmd/connector`，聚合运行时更新为 `ConnectorApp`（不保留任何别名兼容），并通过 Google Wire 生成全新注入代码。
   - `Dockerfile` 与 `docker-compose.yml` 统一更新构建入口为 `SERVICE_PATH: ./cmd/connector`。

3. **传输层监听文件统一为 `http_server.go` 与 `HttpServer`**：
   - 将 `ws_server.go` 重命名为 `http_server.go`，类型对齐为 `HttpServer`，直观反映底层本质就是标准库 `net/http.Server`（监听 HTTP 8000 端口，专职负责 `/ws` WebSocket Upgrade 握手升级、`/health` 探活与 `/metrics` 监控）。
   - 物理删除旧有的 `httpServerShim` 冗余适配器，让 `HttpServer` 直接实现原生 `http.Handler` 接口；与长连接状态生命周期管理器 `ConnectorServer` 及内部通信 `GrpcServerWrapper`（8001）形成清晰的关注点分离。

4. **Protobuf 契约与微服务下推链路完整过度（Breaking Complete Transition）**：
   - 彻底物理删除 `api/proto/gateway.proto` 与生成代码 `api/gen/gateway`；
   - 新建 `api/proto/connector.proto`，声明 `package connector;` 与服务 `service ConnectorService`，编译生成 `kama_chat_server/api/gen/connector`（`connectorpb`）；
   - 全面物理删除所有历史过渡别名（彻底清除 `ChatServer`, `GatewayGrpcServer`, `GatewayApp`, `ProvideChatServer`, `NewGatewayGrpcServer`, `CacheKeyUserGateway` 等）；
   - 基础设施层 `internal/infra/grpc_client` 中的直推方法全量更名为 `GetConnectorClient`、`PushToConnector`、`BatchPushToConnector`，业务微服务（`message`, `friendship`）全量迁移至全新契约。

## 结果
- 彻底清除了仓内所有关于 `gateway`、`ws_gateway`、`ChatServer` 的旧命名遗留与过度别名，IDL、包名、文件名与领域术语完全自洽。
- `docker-compose` 与容器部署镜像编译参数完全对齐为 `cmd/connector`。
- 全仓代码与单元测试全部回归通过。
