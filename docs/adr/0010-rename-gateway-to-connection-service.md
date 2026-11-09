# 0010. 将 WebSocket 网关更名为连接服务 (ConnectionService / Connector)

- **状态**: Accepted
- **日期**: 2026-11-07

## 上下文
在系统先前的微服务与网关拆分演进中，负责维护在线客户端 WebSocket 长连接的模块被习惯性命名为 `gateway`（网关）。然而，随着云原生架构落地（ADR 0005），系统在最前置部署了 **Envoy** 作为正统的 **API 边缘网关（EdgeGateway）**，专职负责外部流量统一入口、SSL 卸载、HTTP 转 gRPC 协议转码（`grpc_json_transcoder`）与路径分流。

在此架构下，内部依然将 WebSocket 服务称作“网关（Gateway）”引发了严重的概念混淆与心智负担：
1. **职责混淆**：Envoy 是无状态的反向代理网关；而该 WebSocket 服务是强业务相关的**有状态长连接管理与推送服务**，二者角色截然不同。
2. **术语冲突**：工程师与文档在提及“网关”时，无法直观区分是指 Envoy 边缘流量网关，还是指内部长连接进程。

## 决策

1. **确立领域命名边界（Domain Boundary）**：
   - **Envoy**：唯一正式称谓为 **`EdgeGateway`（边缘网关 / 流量网关）**。
   - **WebSocket 长连接服务**：正式更名为 **`ConnectionService`（连接服务）**，工程实现中简称为 **`Connector`（连接器）**。
   - 在全工程文档与后续代码中废弃将长连接服务统称为“网关（Gateway）”的用法。

2. **代码与目录渐进式重命名路线（Refactoring Plan）**：
   - **Go 包路径**：由 `internal/gateway` 演进为 `internal/connector`。
   - **gRPC 接口与 Proto**：由 `gatewaypb.GatewayService` 演进为 `connectorpb.ConnectorService`，保持单用户推送 `PushMessage` 与批量推送 `BatchPushMessage` 语义。
   - **服务聚合入口**：由 `ChatServer` / `NewChatServer` 明确为 `ConnectorServer` / `NewConnectorServer`。

3. **存储与路由字段语义对齐**：
   - **分布式路由缓存 Key**：Redis 中的用户在线位置 Key 由 `constants.CacheKeyUserGateway = "user_gateway:"` 平滑演进为 `constants.CacheKeyUserConn = "user_conn:"`。
   - **连接服务门牌地址**：原 `gatewayGrpcAddr` 统一重命名为 `connGrpcAddr`，清晰表明这是连接服务自身的内部 gRPC 通信地址。

## 结果
- 彻底消除了系统拓扑中“双网关”的名词歧义，使系统分层概念（`边缘接入 Envoy` $\rightarrow$ `长连接维持 Connector` $\rightarrow$ `业务微服务 Kratos`）高度清晰、符合工业界标准。
- 在 `GLOSSARY.md` 与 ADR 中固化了领域词汇，为后续代码重构提供了权威依据。
