# 0011. 连接服务采用 coder/websocket 替代 nbio Multi-Reactor

- **状态**: Accepted
- **日期**: 2026-11-07

## 上下文
在先前的设计中，连接服务（`ConnectionService`）采用了 `github.com/lesismal/nbio` 提供的 Multi-Reactor 非阻塞网络事件引擎。其初衷是为了压榨单机对海量挂机长连接（C100K~C1000K）的极致内存利用。

然而在当前的分布式云原生微服务体系下，存在以下技术演进现实与痛点：
1. **网络模型复杂度与生态脱节**：`nbio` 拥有一套独立的底层事件循环系统（`nbhttp.Engine` / `nbhttp.Server`），与 Go 标准库 `net/http` 及其生态中间件脱节，提升了代码的维护心智与调试门槛；
2. **Context 生命周期控制缺失**：`nbio` 许多底层读写操作缺乏原生 `context.Context` 深度集成；
3. **容量定位澄清**：在当前微服务分片架构下，单个 Pod 节点连接规模规划在数万以内，无需强行引入复杂的 C1000K 级自定义 Reactor 引擎，标准库的协程模型完全能够轻松支撑，且具备更高的可读性与健壮性。

## 决策

1. **底层传输库切换为 `github.com/coder/websocket`**：
   - 移除 `github.com/lesismal/nbio`，全面拥抱由 Coder 团队维护的官方推荐现代 WebSocket 库 `coder/websocket`（原 `nhooyr.io/websocket`）。
   - 服务端监听回归 Go 标准库 `net/http.Server`，握手过程使用原生 `websocket.Accept(w, r, opts)`。

2. **保留应用层非阻塞背压通道设计（SendBack chan）**：
   - 虽然 `coder/websocket` 在底层支持多协程并发安全调用 `conn.Write()`，但其内部是基于互斥锁（`sync.Mutex`）实现的串行化写入。
   - 为彻底防止慢客户端/弱网连接因底层写入阻塞而造成上游 gRPC 推送 Handler 协程堆积与雪崩，`UserConn` **必须继续保留有界容量为 100 的 `SendBack chan`**；
   - 业务层只调用非阻塞的 `SafeSend`，队列满时主动丢弃实时下行信令并告警，由各连接的单向写协程调用 `conn.Write(ctx, ...)` 执行真正的网络推送。

3. **读写分离与 Goroutine-per-Connection 生命周期管理**：
   - 连接握手成功后，由专职 `readPump` 协程负责循环阻塞调用 `conn.Read(ctx)` 解析客户端上行帧与心跳；
   - 连接生命周期由原生的 `context.WithCancel` 进行统一级联管理，连接注销时通过 `conn.Close(websocket.StatusNormalClosure, "closing")` 优雅释放。

## 结果
- 架构彻底回归 Go 标准库纯粹生态，消除了第三方自定义事件引擎的黑盒复杂度；
- 原生享受 `context.Context` 的超时控制、追踪透传与优雅退出；
- 配合“推拉结合（Push-Pull）”增量同步机制，既保全了连接服务免于慢连接协程雪崩，又确保了离线/弱网消息恢复后的最终一致性。
