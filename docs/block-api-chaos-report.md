# 好友拉黑接口（`PUT /friends/block`）弱网混沌验证报告

> 验证方式：在本地 k3s（WSL2）起全量微服务栈，用 Toxiproxy 对真实链路注入延迟/断连，
> 以黑盒方式（真 HTTP → Envoy → gRPC → MySQL）验证拉黑接口的正确性。
> 全部场景代码在 `tests/e2e/`，部署与运行脚本在 `scripts/k3s/`。

---

## 0. 结论速览

| 维度 | 结论 |
|---|---|
| 拉黑接口的**数据一致性**（弱网下是否半拉黑） | 见下方场景矩阵，核心断言为「全成功或全回滚」 |
| 拉黑的**幂等性** | 见 B1 |
| **ABA 防御** | **失效** —— API 层无法表达版本号，服务端无从区分陈旧请求 |
| **越权** | **存在** —— Envoy 路径无认证，请求体里的 `user_id` 被无条件信任 |
| **错误可观测性** | **不可判别** —— 业务错误码在 gRPC 边界全部退化为「服务繁忙」 |
| 前置链路（好友申请/审批） | **完全不可用**，两个独立的 P0 缺陷 |
| 服务发现 | **服务重启后永久失效**（注册键无实例唯一性） |

---

## 1. 验证环境

```
Windows 11 (22631) / 32GB
└─ WSL2 Ubuntu-24.04 (systemd)
   └─ k3s v1.36.5+k3s1 (--disable traefik --disable servicelb --disable metrics-server)
      ├─ MySQL 8.0.46   (chat_user/chat_apply/chat_friendship/chat_group/chat_message + 6 个最小权限账号)
      ├─ Redis 7        ├─ Kafka 4.1.1 (KRaft)  ├─ etcd 3.5.17
      ├─ chat-server(Gin 网关) user_service apply_service friendship_service
      │  group_service message_service cdc_service envoy
      └─ toxiproxy 2.11 (3 条代理路由) + e2e Job
```

**Toxiproxy 插桩点**（零 Go 代码改动）

| 代理 | 监听 | 上游 | 覆盖的链路 |
|---|---|---|---|
| `mysql_proxy` | 33066 | `mysql:3306` | friendship_service → MySQL（拉黑写事务的 7 个往返） |
| `redis_proxy` | 63855 | `redis:6379` | friendship_service → Redis |
| `friendship_grpc` | 50054 | `friendship-service-real:50054` | 所有 gRPC 客户端 → friendship_service |

> gRPC 跳之所以能零代码插桩：`friendship_service` 把自身地址以**服务名**形式注册到
> etcd（`cmd/friendship_service/main.go` 硬编码 `friendship-service:50054`），
> 因此劫持同名 k8s Service 的 selector 即可让 Envoy / Gin 网关 / apply / message
> 的全部 gRPC 调用自动经过代理。

---

## 2. 场景矩阵结果

<!-- RESULTS_PLACEHOLDER -->

---

## 3. 缺陷清单

### 3.1 P0 —— 正确性 / 安全 / 数据一致性

#### P0-1　服务发现：服务重启后 gRPC 调用永久失效

- **现象**：任何服务滚动重启后，调用方报
  `rpc error: code = Unavailable desc = no children to pick from`，接口表现为「服务繁忙」。
- **根因**：`pkg/discovery/register.go` 的注册键是 `<name>/<addr>`，**不含实例唯一性**。
  k8s 滚动更新时新 Pod 先起（maxSurge）旧 Pod 后退，旧 Pod 退出时 `Stop()` 执行
  `DeleteEndpoint("<name>/<addr>")`，正好把新 Pod 刚注册的同一个键删掉，且新 Pod 不会补注册。
- **影响**：任何一次滚动更新，都会让被重启服务的所有上游调用方永久失联。
- **修复**：注册键追加租约 ID（每实例唯一），一个实例下线不会误删其它实例的注册。
  已随本次验证修复（`pkg/discovery/register.go`）。

#### P0-2　好友申请接口 100% 不可用（空指针 panic）

- **现象**：`POST /friends/apply` 恒返回 `{"code":1005,"msg":"服务繁忙"}`。
- **根因**：`cmd/apply_service/main.go` **完全没有调用 `grpc_client.Init(...)`**，
  导致包级变量 `grpc_client.UserClient` 恒为 nil；
  `internal/apps/apply/service.go:89` 的 `grpc_client.UserClient.GetUserStatus(...)`
  对 nil 接口取值 → panic → 被 Kratos recovery 兜住 → 客户端只看到 Internal/Unknown。
- **修复**：补上 `grpc_client.Init([]string{"etcd:2379", "127.0.0.1:2379"})`（已修）。

#### P0-3　好友申请接口 100% 不可用（枚举错配）

- **现象**：修掉 P0-2 后，申请依然被拒，错误信息变成 **「你们已经是好友」**——
  而这对用户是刚注册的。
- **根因**：`internal/apps/apply/service.go` 拿**对外契约状态码**去比**数据库行状态枚举**：
  ```go
  // CheckFriendshipStatus 的契约：0=非好友 1=正常好友 2=已拉黑 3=被拉黑 4=单向删除
  // friendship_status 枚举：  0=NORMAL   1=BE_BLACK 2=BLACK
  if statusVal == friendship_status.NORMAL {   // 等价于 statusVal == 0
      return ... "你们已经是好友"
  }
  ```
  两套编码的 `0` 含义完全相反 → **任何一对非好友都被判成「已经是好友」**。
- **影响**：好友申请链路 100% 不可用，拉黑接口的前置条件无法通过真实链路构造。
- **修复**：改用契约码 `1`，并加注释说明两套编码的区别（已修）。

#### P0-4　Envoy 路径可越权：请求体里的 `user_id` 被无条件信任

- **现象**：`PUT /friends/block`，请求体写 `{"user_id":"<受害者>","friend_id":"X"}`，
  **不带任何 token**，即可把受害者拉黑。
- **根因**（三处叠加）：
  1. `configs/envoy.yaml` 的过滤器链只有 `cors → grpc_json_transcoder → router`，
     **没有任何认证过滤器**；Envoy 既不注入也不剥离 `x-user-id`，
     CORS 的 `allow_headers` 里还明确列了 `x-user-id`。
  2. `internal/apps/friendship/grpc_server.go:43` **优先信任 `req.UserId`**（来自请求体），
     `X-User-Id` 只是 fallback。
  3. 没有任何服务端拦截器覆盖 `req.UserId`。
- **对照**：`cmd/gateway` 的 Gin 路径是安全的 —— `user_id` 只来自验签后的 JWT，
  请求 DTO 里根本没有 `user_id` 字段。
- **修复方向**：在边缘加真正的认证（Envoy `jwt_authn` + `ext_authz`，或 APISIX `jwt-auth`）
  并在服务侧改为只信任受信头；或用 transcoder 的 `proto_api_scrubber` 剥离请求体里的 `user_id`。

#### P0-5　ABA 防御在 API 层不可达

- **现象**：`拉黑 → 解拉黑 → 迟到的旧拉黑包` 之后，好友关系从 NORMAL **倒退回 BLACK**。
- **根因**：`api/proto/friendship.proto` 的 `BlackFriendRequest` 只有 `user_id` / `friend_id`，
  **没有 version 字段**；`grpc_server.go` 调 `BlackFriend(ctx, userId, friendId)`
  不传 `expectedVersion`；`internal/apps/friendship/service.go` 里那段
  「读 version → 不一致则 `CodeConflict(1013)`」的 ABA 防线**在真实 API 上永远走不到**。
- **对照**：`tests/chaos/chaos_test.go` 的 ABA 用例是**直接调领域层并手动传 `version=1`**
  才验到拦截的 —— 它验证的是领域层，不是接口。
- **修复方向**：proto 加 `version` 字段并一路透传（会牵动 `api/gen/*.pb.go`
  与 `configs/proto.pb` 的重新生成，建议独立 PR）。

#### P0-6　业务错误码在 gRPC 边界全部丢失

- **现象**：任何业务错误（1001 参数错 / 1007 无权限 / 1013 冲突）在两条路径上
  都只能看到「服务繁忙」。
- **根因**：
  1. `pkg/errorx` 的 `CodeError` **没有实现 `GRPCStatus()`**，
     grpc-go 的 `status.FromError` 一律返回 `codes.Unknown(2)`，
     业务码 1001/1007/1013 在链路上彻底消失。
  2. Gin 网关把 `*status.Error` 交给 `HandleError`，
     `errors.As(err, &codeErr)` 匹配不上 → 落到「未知错误」分支 → 统一返回 1005「服务繁忙」。
  3. Envoy 侧 `convert_grpc_status: true` 把 `Unknown` 渲染成
     **HTTP 500** + `{"code":2,"message":"<中文 msg>"}`。
- **结果**：客户端无法区分「不能拉黑自己」「你们还不是好友」「数据库挂了」，
  只能靠中文 message 猜测。
- **修复方向**：给 `CodeError` 实现 `GRPCStatus()`，把业务码映射到合适的 gRPC code；
  网关侧改用 `errors.As` 之外的结构化错误传递。

### 3.2 P1 —— 契约与可观测性

#### P1-1　Envoy 路由表按「转码后的 gRPC 路径」匹配，apply/group/message 全部失配

`grpc_json_transcoder` 默认用转码后的路径（`/apply.ApplyService/ApplyFriend`）
去匹配路由表，而配置里的 prefix 是按 HTTP 语义写的（`/friends/appl`），
于是这些请求全部落到 `/` 兜底路由 → 打到 `chat-server`。
`/friends/block` 只是因为 gRPC 包名恰好也叫 `friendship` 才碰巧命中。

**修复**：`match_incoming_request_route: true`（已修）。

#### P1-2　Envoy 配置有两处会让它直接启动失败

- `retry_back_off.base_interval: 25ms` / `max_interval: 250ms`
  —— protobuf Duration 只接受以单个 `s` 结尾的秒数形式，
  报错 `duration must end with a single 's'`。
- `upgrade_configs` 写在了 `route:` 外面（它属于 RouteAction）——
  报错 `no such field: 'upgrade_configs'`。

（两处均已修）

#### P1-3　客户端熔断器把业务错误当成故障

`pkg/interceptor/breaker.go` 的 `IsSuccessful` 只把
`OK/InvalidArgument/NotFound/AlreadyExists/PermissionDenied/Unauthenticated/FailedPrecondition`
算作成功。由于 P0-6，**所有业务错误都是 `codes.Unknown`** → 全部计入失败 →
**连续 5 次业务失败就打开熔断器**，之后 10 秒内该方法的调用一律
`Unavailable`（Envoy 503 / 网关 1005「服务繁忙」）。

一个「重复拉黑」类业务错误就能把整个拉黑接口打成不可用。

#### P1-4　CDC 遇 Kafka 故障直接死掉且不重启

- `cmd/cdc_service/main.go:98`：CDC worker 出错后只 `zap.L().Error` 就退出，
  **进程还活着但 worker 已死**，没有 supervisor、没有重启。
- `[kafkaConfig] timeout = 1` → franz-go 的 `RecordDeliveryTimeout` 只有 **1 秒**，
  Kafka 抖动 1 秒以上就会触发上面的死亡路径。
- 叠加 P0-6：CDC 发布失败时 `OnRow` 返回 error → canal 同步循环中止 → 进程内不再消费。

#### P1-5　WS 通知推送会静默丢失

`internal/apps/message/message/session_event_handler.go` 的 `pushNotice`：
Redis 里查不到 `user:gateway:<id>` 就 **静默 return**（无错误、无日志、无指标），
而位点照常提交 —— 通知丢了也看不出来。

#### P1-6　会话软删其实是物理删除

`internal/common/dao/mysql/session/session_store.go` 的 `SoftDeleteByUuids`
用的是 `Unscoped().Delete(...)` → **行被真删，`deleted_at` 永不置位**。
方法名与实际行为不符，断言必须写成「行数为 0」而不是「`deleted_at IS NOT NULL`」。

#### P1-7　DB 列注释与 Go 枚举相反

`internal/common/model/friendship.go` 的列注释写「0.正常，1.拉黑，2.被拉黑」，
而 `pkg/enum/friendship/friendship_status` 是 `NORMAL=0 / BE_BLACK=1 / BLACK=2`。
代码以 Go 枚举为准，**注释是错的**。拉黑后 A 的行是 `status=2`、B 是 `status=1`。

### 3.3 部署侧（`deploy/k8s/`）修复清单

| 问题 | 影响 | 状态 |
|---|---|---|
| MySQL 缺 `--log-bin/--binlog-format=ROW/--gtid-mode=ON` | CDC 完全无法启动 | 已修 |
| MySQL 缺 `/docker-entrypoint-initdb.d` 挂载 | 5 个拆分库与 6 个服务账号不存在 | 已修 |
| MySQL 用了 `command: ["mysqld"]` | **覆盖了镜像的 ENTRYPOINT**，初始化脚本与 initdb 全不执行，mysqld 直接对着空 datadir 启动并崩溃 | 已修（改用 `args:`） |
| `message-service` 占用 50054 | 与 friendship_service 冲突（实际是 50056） | 已修 |
| 存在 `auth-service`(50052) / `relation-service`(50053) 两个幽灵 Deployment | 指向不存在的服务 / 名字过时 | 已修（删除并改为 apply-service） |
| 缺 apply/friendship/group/cdc/envoy 的 Deployment | 集群跑不起来 | 已补 |
| `chat-static-pvc` 用 `ReadWriteMany` | local-path 只支持 RWO，PVC 永远 Pending | 已修 |
| Secret 里 `REDIS_PASSWORD:` 是 YAML null | 引用它的 Deployment 全部 `CreateContainerConfigError` | 已修 |
| Kafka PVC 挂在 `/tmp/kafka-logs` | 镜像实际用 `/tmp/kraft-combined-logs`，数据落容器可写层 | 已修 |
| etcd 广播 `http://0.0.0.0:2379` | 客户端跟随广播地址会连回环 | 已修 |
| 无 CDC Deployment / 无 toxiproxy 代理路由 | 异步半程与故障注入无从谈起 | 已补 |

---

## 4. 复现方式

```powershell
# 1. 让 WSL2 常驻（否则 WSL 会反复 poweroff，k3s 跟着反复重启）
pwsh -File scripts/k3s/wsl-keepalive.ps1

# 2. 构建镜像（宿主机交叉编译 + WSL 内 nerdctl 打包进 k3s 的 containerd）
pwsh -File scripts/k3s/build-images.ps1

# 3. 部署（顺序敏感，每一步都对应一个「顺序错了就会得出假结论」的坑）
wsl -d Ubuntu-24.04 -u root -- bash /mnt/d/repo/chat-server/scripts/k3s/deploy.sh

# 4. 跑 e2e
wsl -d Ubuntu-24.04 -u root -- bash /mnt/d/repo/chat-server/scripts/k3s/run-e2e.sh

# 5. 人工核对
kubectl -n chat-system port-forward svc/envoy 9080:9080
kubectl -n chat-system port-forward svc/toxiproxy 8474:8474
```

`scripts/k3s/` 下另有 `reset.sh`（清空环境）、`clean-pvc.sh`、
`setup-registry-mirror.sh`（本机 docker.io 的 DNS 被污染，需要加速器）。

---

## 5. 遗留事项（本次未修，按 Q12「不改业务 Go 代码」的约定交后续 PR）

| # | 事项 | 涉及文件 |
|---|---|---|
| 1 | 边缘加认证 + 服务侧只信任受信头 | `configs/envoy.yaml`、`internal/apps/*/grpc_server.go` |
| 2 | proto 增加 `version` 字段并透传，启用 ABA 防线 | `api/proto/friendship.proto`、`api/gen/**`、`configs/proto.pb` |
| 3 | `errorx.CodeError` 实现 `GRPCStatus()`，网关侧保留业务码 | `pkg/errorx/errorx.go`、`internal/apps/gateway/handler/response.go` |
| 4 | 熔断器把 `codes.Unknown` 从「失败」里排除 | `pkg/interceptor/breaker.go` |
| 5 | CDC 增加退避重启；Kafka delivery timeout 放大 | `cmd/cdc_service/main.go`、`configs/config.toml` |
| 6 | `pushNotice` 失败要打日志/指标 | `internal/apps/message/message/session_event_handler.go` |
| 7 | 修正 DB 列注释、明确 `SoftDeleteByUuids` 的真实语义 | `internal/common/model/friendship.go`、`internal/common/dao/mysql/session/session_store.go` |
| 8 | `ETCD_ENDPOINTS` 环境变量未被任何代码读取 | `cmd/*/main.go` |
