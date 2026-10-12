# 0006. RESTful HTTP 动词精细化重构 (POST 转 PUT 规范对齐)

- **状态**: Accepted
- **日期**: 2026-10-10

## 上下文
在系统从手写网关演进为纯 gRPC + Envoy `grpc_json_transcoder` 架构后，全面梳理了系统中原有的 19 个 `POST` 接口。需要明确 HTTP 动词边界：哪些写操作属于天然幂等的属性/状态设置，应使用 `PUT`；哪些属于复杂副作用或创建操作，应保持 `POST`。

## 决策
经过 Grilling 逐项推导，遵循 **Google Cloud API AIP-136 规范** 与 **RESTful 幂等语义** 进行分类调整：

1. **调整为 `PUT` 的天然幂等属性/状态设置操作（共 2 个）**：
   - `PUT /friends/block`（拉黑好友）：与 `DELETE /friends/block`（解除拉黑）形成标准的对称黑名单属性设置，语义为覆盖状态。
   - `PUT /groups/members/mute`（群成员禁言）：设置群成员禁言时间戳属性，覆盖更新，多次执行具有天然幂等性。
2. **严格保持 `POST` 的核心创建与动作命令操作（共 17 个）**：
   - **资源创建与会话开启（5个）**：`POST /auth/register`、`POST /groups`、`POST /friends/apply`、`POST /groups/apply`、`POST /sessions`（打开/创建会话：虽在已存在时只查不建，但首次调用具备显式创建新实体与分配存储资源的工厂副作用，遵从创建语义规范）。
   - **安全与凭据命令（2个）**：`POST /auth/login`、`POST /auth/logout`（严禁幂等回放，每次重新生成加密上下文）。
   - **业务审批流动作（6个）**：`POST /friends/applies/approve`、`refuse`、`block`；`POST /groups/applies/approve`、`refuse`、`block`。这些操作具有状态机流转、发系统通知等复杂业务副作用，符合 Custom Method 规范。
   - **消息动作与群管理（3个）**：`POST /messages/recall`（消息撤回动作）、`POST /groups/leave`（主动退群动作）、`POST /auth/kick`（强制踢出管理动作）。
   - **批量查询（1个）**：`POST /user/public-info/batch`（由于 URL Query 长度限制，使用 POST body 传参的批量读取）。

## 结果
- HTTP 动词语义高度贴合云原生与 RESTful 工业级规范。
- 彻底消除了前端对拉黑、禁言等接口“连击是否会导致脏数据”的顾虑。
- 重新编译生成了最新的 `configs/proto.pb`，Envoy 网关零代码变更即可自动适配新协议路由。
