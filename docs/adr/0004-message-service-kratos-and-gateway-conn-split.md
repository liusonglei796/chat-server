# 0004. MessageService Kratos 双协议改造与网关专职长连接 ConnService 演进

- **状态**: Accepted
- **日期**: 2026-10-10

## 上下文
在微服务治理基础设施替换的既定方案中，前序服务（`user_service`, `friendship_service`, `apply_service`, `group_service`）已全部完成 Bilibili Kratos v2 双协议化。核心系统仅剩 `message_service` 仍依赖手工编写的旧 gRPC 拦截器与注册器，且网关仍承担着大量的 Gin BFF 胶水路由逻辑。

## 决策
1. **MessageService Kratos v2 升级**：
   - 在 `api/proto/message.proto` 引入 `google.api.http` 注解，声明 `/sessions*` 与 `/messages*` RESTful 规范，自动生成 `message_http.pb.go`。
   - `message_service` 同时暴露 gRPC (50054) 与 HTTP (8006)，挂载标准 JSON 响应包装器与错误响应编码器，对齐 `code: 1000` 协议规范。
   - 实现受信身份下沉：直接从 `X-User-Id` 请求头或 Context 绑定用户上下文，省去重复验签计算。
2. **APISIX 全链路路由接管**：
   - 在 `configs/apisix_routes.yaml` 中将 `/sessions*` 与 `/messages*` 直连反代至 `message-service:8006`。
   - 配置 `/ws*` 长连接专属路由，启用 `enable_websocket: true`，转发至长连接服务。
3. **复合网关瘦身为专职 ConnService**：
   - 彻底卸载所有 RESTful BFF 路由转发职责。
   - 专职承载有状态 WebSocket 连接接入（支持 APISIX `X-User-Id` 受信握手与 URL token 兼容）、心跳监测及 `GatewayService` 下行消息直推。
4. **集成测试全量验证**：
   - 升级 `test_all_apis.py` 测试套件，端到端穿透 APISIX (9080) 覆盖全链路（用户注册登录 -> 资料获取 -> 好友申请审批 -> 群组创建加入 -> 会话开启检查 -> 消息历史查询）。

## 结果
- 淘汰了自研的手写微服务路由与网关胶水代码，微服务治理全面转向 Kratos 官方 SDK。
- 公网接入层由工业级 Apache APISIX 全权负责负载均衡与反代，长连接接入与下推由精简后的 ConnService 专注处理，架构清晰解耦。
