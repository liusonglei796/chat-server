# Redis 异步任务池（ants Worker Pool）使用指南

> 本文档教你在 chat-server 中如何使用 Redis 异步缓存任务池。
> 该池基于 [ants](https://github.com/panjf2000/ants)（14.5k+ star）实现，
> 用于把"缓存写入/失效"这类不关键的 Redis 操作从请求路径上剥离，异步执行，提升接口吞吐。

---

## 1. 它解决什么问题

场景：用户改了个资料，业务代码要同时做两件事——

1. 写 MySQL（关键，必须同步，保证用户看到最新数据）
2. 清掉 Redis 里的旧缓存（不关键，晚 100ms 清也无所谓）

如果第 2 步也同步做，接口延迟就被 Redis 往返拖累；如果每处都 `go func()` 裸开 goroutine，
突发流量下 goroutine 无界增长。**异步任务池**就是折中方案：**有限的 Worker（默认 15 个）并发消费任务队列，池满自动降级**。

```
HTTP 请求 ──> Service 业务逻辑
                  │
                  ├─ 同步：写 MySQL（关键路径，立即执行）
                  │
                  └─ 异步：SubmitTask(清缓存) ──> ants 池（15 个 Worker）
                                                      │
                                                      └─ 逐个执行缓存清理
```

## 2. 核心 API

接口定义在 `internal/domain/repository/cache.go`：

```go
type AsyncCacheService interface {
	CacheService                       // 同步缓存操作：Set/Get/Delete/AddToSet...
	SubmitTask(action func())          // 异步：把任务交给 worker 池
}
```

你**只需要记住一个方法**：`SubmitTask(func(){ ... })` —— 把一个闭包扔进池子，立即返回，不阻塞。

## 3. 快速上手（三步）

### 第一步：拿到缓存服务

Service 构造时已经注入好了。两种拿法：

```go
// 方式 A：整个注入（事务型 Service 的通用模式）
type GroupService struct {
	uow    repository.UnitOfWork
	cache  repository.AsyncCacheService   // ← 这就是池的入口
}

// 方式 B：只注入缓存接口（非事务型 Service）
func NewSessionService(cache repository.AsyncCacheService, ...) *SessionService
```

### 第二步：提交异步任务

```go
// 业务操作完成后，把缓存清理丢进池
if err := g.uow.WithTx(func(tx repository.UnitOfWork) error {
	return tx.GroupRepo().UpdateGroup(ctx, &group)
}); err != nil {
	return errorx.ErrServerBusy
}

// 关键路径走完，缓存失效交给异步池
g.cache.SubmitTask(func() {
	if err := g.cache.Delete(context.Background(), constants.CacheKeyGroupInfo+groupId); err != nil {
		zap.L().Error("清理群信息缓存失败", zap.Error(err))
	}
})
return nil
```

### 第三步：进程退出时释放池

池在收到退出信号时优雅关闭，等已提交的任务跑完再退出，不丢任务。
这个已经在 5 个服务的 main 里接好了，**你写业务代码不用管**：

```go
// cmd/*/main.go（已内置）
<-quit
if rc, ok := cacheService.(*myredis.RedisCache); ok {
	rc.Release()   // 等待任务执行完毕，然后关闭 Worker
}
```

---

## 4. 真实代码示例

### 示例 1：简单失效（`internal/service/group/service.go:244`）

群信息修改后，异步清理群信息 + 群成员两个缓存：

```go
g.cache.SubmitTask(func() {
	if err := g.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyGroupInfo+groupId); err != nil {
		zap.L().Error("清理群信息缓存失败", zap.Error(err))
	}
	if err := g.cache.Delete(context.Background(), constants.CacheKeyGroupMembers+groupId); err != nil {
		zap.L().Error("清理群成员缓存失败", zap.Error(err))
	}
})
```

### 示例 2：读-改-写（`internal/service/message/kafka_processor.go:428`）

消息处理时异步更新"消息列表"缓存——读取旧列表、追加新消息、写回：

```go
k.cacheService.SubmitTask(func() {
	id1, id2 := message.SendId, message.ReceiveId
	if id1 > id2 {
		id1, id2 = id2, id1
	}
	key := constants.CacheKeyMessageList + id1 + "_" + id2
	rspString, err := k.cacheService.GetOrError(context.Background(), key)
	if err == nil {
		var list []messagersp.GetMessageListRespond
		if err := json.Unmarshal([]byte(rspString), &list); err == nil {
			list = append(list, messageRsp)
			if rspByte, err := json.Marshal(list); err == nil {
				_ = k.cacheService.Set(context.Background(), key, string(rspByte), time.Minute*constants.REDIS_TIMEOUT)
			}
		}
	}
})
```

---

## 5. 内部机制（ants 池怎么工作）

实现位于 `internal/dao/redis/redis.go`：

```go
type RedisCache struct {
	client *redis.Client
	pool   *ants.Pool   // ants goroutine 池
}

func NewRedisCache(client *redis.Client, poolSize int) *RedisCache {
	pool, err := ants.NewPool(poolSize,
		ants.WithNonblocking(true),   // Submit 永不阻塞调用方
		ants.WithPanicHandler(func(p any) {
			zap.L().Error("Redis cache task panic", zap.Any("panic", p))
		}),
	)
	if err != nil {
		zap.L().Fatal("create ants pool failed", zap.Error(err))
	}
	return &RedisCache{client: client, pool: pool}
}

func (r *RedisCache) SubmitTask(action func()) {
	if err := r.pool.Submit(action); err != nil {
		if errors.Is(err, ants.ErrPoolOverload) {
			zap.L().Warn("Redis cache pool overloaded, executing synchronously")
			action()   // 池满 → 降级为同步执行，宁可慢也不丢
		}
		// ants.ErrPoolClosed: 进程退出中，任务丢弃
	}
}
```

三个关键行为：

| 行为 | 机制 | 后果 |
|---|---|---|
| **并发上限** | `ants.NewPool(15)` | 最多 15 个任务同时执行，goroutine 复用不反复创建 |
| **池满降级** | `WithNonblocking(true)` + `ErrPoolOverload` | Submit 不阻塞调用方，池满时任务转同步执行 |
| **任务 panic** | `WithPanicHandler` | panic 被 recover 并记日志，Worker 不退出，其他任务不受影响 |

---

## 6. 配置：改 Worker 数量

在 `internal/dao/redis/redis.go` 的 `Init()` 里：

```go
return NewRedisCache(client, 15)   // 15 = Worker 数量
```

**选多大合适：**

- 任务都是**快速的 Redis 操作**（毫秒级），Worker 越大并发越高，但也会占用更多 Redis 连接
- 项目里 Redis 连接池是 `PoolSize: 50, MinIdleConns: 15`——**Worker 数建议不超过连接池的 MinIdle（15）**，否则 Worker 会排队抢连接
- 常规场景 15 足够；如果发现 `"pool overloaded, executing synchronously"` 日志频繁出现，再考虑调大

---

## 7. 使用规则与最佳实践

### ✅ 必须遵守

1. **任务里用 `context.Background()`，不要用请求的 `ctx`**
   任务被延迟执行，请求的 ctx 可能已被取消（超时/客户端断开），会导致任务里的 Redis 操作全部失败。
   示例里全部是 `context.Background()`，照抄即可。

2. **任务要短、要快**
   这是缓存专用通道，别往里面丢耗时任务（如 HTTP 调用、批量大查询）。耗时操作会占满 Worker，把其他缓存失效全部挤到降级路径。

3. **闭包内处理错误并记日志**
   `SubmitTask` 不返回任务结果，错误只能靠闭包内部处理：

   ```go
   g.cache.SubmitTask(func() {
       if err := g.cache.Delete(ctx, key); err != nil {
           zap.L().Error("清理缓存失败", zap.Error(err))
       }
   })
   ```

4. **任务要幂等**
   缓存失效天然幂等（删不存在的 key 也没事），但如果你是"读改写"类任务（如示例 2），
   要考虑并发执行两次会不会产生脏数据。ants 不保证同 key 任务的顺序。

### ⚠️ 不要做

5. **不要把关键路径操作丢进池**
   比如"用户注册后写初始数据"——用户等不起，也接受不了丢失。只有"不关键、晚点做也行"的操作才异步化。

6. **不要依赖任务执行完成的时序**
   异步意味着"不确定何时执行"。如果后续代码读的是刚失效的缓存，可能读到旧值——用 `context` 传依赖的调用链设计要绕开这个窗口。

7. **不要在任务里再调 `SubmitTask`（嵌套）**
   会占用额外 Worker 且难以追踪。直接在一个任务里做完即可。

---

## 8. 常见问题 FAQ

**Q: 池满了会发生什么？**
任务转为同步执行（`SubmitTask` 里直接 `action()`），记一条 Warn 日志。不丢任务，但接口延迟会上升——这是有意的"慢一点 vs 丢数据"取舍。

**Q: 进程退出时任务会丢吗？**
不会。5 个 main 都在退出路径调了 `rc.Release()`，ants 会等已提交任务执行完再关闭。`Release` 之后新提交的任务会收到 `ErrPoolClosed` 被丢弃（此时进程已在退出）。

**Q: 为什么不用 `go func()` 裸开 goroutine？**
裸开没有并发上限——突发流量下 goroutine 无限增长，最终打爆内存或 Redis 连接池。池的作用就是"限流 + 复用"。

**Q: 为什么任务里报"panic"不会搞挂服务？**
`WithPanicHandler` 捕获了 panic 并记日志，Worker 继续复用。但注意：**panic 所在任务本身已经丢了**，所以任务逻辑里还是要自己处理错误。

**Q: 这个池和 Kafka 消费是什么关系？**
两回事。Kafka 消费（`kafka_processor.go`）是**消息驱动的并发**（一个消息一个 goroutine，来自 Kafka 消费者组）；这个池是**任务队列的并发**（Service 主动提交的缓存操作）。别混淆。

---

## 9. 文件索引

| 文件 | 角色 |
|---|---|
| `internal/dao/redis/redis.go` | 池的实现（ants 封装 + SubmitTask + Release） |
| `internal/domain/repository/cache.go` | `AsyncCacheService` 接口定义 |
| `internal/service/group/service.go` | 使用示例（异步失效缓存） |
| `internal/service/message/kafka_processor.go` | 使用示例（异步读改写缓存） |
| `cmd/*/main.go`（5 个） | 优雅关闭接线（`rc.Release()`） |

## 10. 相关文档

- [Unit of Work 模式与回调机制解析](uow-pattern.md) —— 事务侧的多仓库协调
- ants 官方文档：<https://github.com/panjf2000/ants>
