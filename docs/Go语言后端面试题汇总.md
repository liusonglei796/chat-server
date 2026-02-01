# Go语言后端面试题汇总

> 本文档整理自小红书及其他平台关于Go语言后端开发的面试题，包含详细答案解析

## 目录

- [Go语言基础](#go语言基础)
- [并发与调度](#并发与调度)
- [数据结构与集合](#数据结构与集合)
- [内存管理与GC](#内存管理与gc)
- [数据库与缓存](#数据库与缓存)
- [网络与框架](#网络与框架)
- [算法题](#算法题)
- [项目与职业规划](#项目与职业规划)

---

## Go语言基础

### 1. = 和 := 的区别？ 🌟

**答案**：
- `=` 是赋值操作符，用于给已声明的变量赋值
- `:=` 是短变量声明，用于声明并初始化变量

```go
var a int
a = 10        // 赋值
b := 20       // 声明并初始化
```

### 2. Go 有异常类型吗？

**答案**：
Go 没有 try-catch 异常机制，使用 `error` 类型代替：
- Go 通过返回 `error` 类型来处理错误
- 可以使用 `errors.New()` 自定义错误
- 通过 `panic/recover` 处理严重错误

```go
func divide(a, b int) (int, error) {
    if b == 0 {
        return 0, errors.New("division by zero")
    }
    return a / b, nil
}
```

### 3. 什么是协程（Goroutine）？

**答案**：
Goroutine 是 Go 语言的轻量级线程，特点包括：
- 用户态线程，由 Go 运行时调度
- 初始栈大小仅 2KB-4KB，可动态伸缩
- 可以轻松创建成千上万个 goroutine
- 使用 `go` 关键字启动

```go
go func() {
    fmt.Println("Hello from goroutine")
}()
```

### 4. 如何高效地拼接字符串？ 🌟（小红书）

**答案**：
性能从高到低：
1. `strings.Builder` - 推荐，零拷贝，预分配内存
2. `bytes.Buffer` - 底层是 `[]byte`
3. `fmt.Sprintf` - 通用但性能一般
4. `+` 操作符 - 每次都创建新对象

```go
// 推荐
var builder strings.Builder
builder.Grow(100) // 预分配
builder.WriteString("Hello")
builder.WriteString(" World")
result := builder.String()
```

### 5. 什么是 rune 类型？ 🌟（小红书）

**答案**：
- `rune` 是 `int32` 的别名
- 用于表示 Unicode 码点
- Go 字符串底层是 `[]byte`（8位），`routine` 是 32 位
- 处理中文等非 ASCII 字符时需要使用

```go
str := "你好"
fmt.Println(len(str))      // 6 (字节长度)
fmt.Println(len([]rune(str))) // 2 (字符数量)
```

### 6. defer 的执行顺序？ 🌟（小红书）

**答案**：
- 执行顺序与调用顺序相反（LIFO - 后进先出）
- 在 `return` 之前执行
- 可以修改具名返回值

```go
func example() (x int) {
    defer func() { x++ }()
    return 1  // 实际返回 2
}
```

### 7. new 和 make 的区别？ 🌟（小红书）

**答案**：
| 特性 | new | make |
|------|-----|------|
| 用途 | 分配内存 | 创建并初始化 slice、map、channel |
| 返回值 | 类型 `*T` 的指针 | 引用类型本身 |
| 初始化 | 零值 | 根据类型初始化 |

```go
p := new(int)       // *int，值为 0
s := make([]int, 5) // []int，有 5 个元素
```

### 8. 数组和切片的区别？ 🌟（小红书外派、优咔科技）

**答案**：
| 特性 | 数组 | 切片 |
|------|------|------|
| 长度 | 固定 | 可变 |
| 传递 | 值拷贝 | 引用传递 |
| 类型 | 长度是类型的一部分 | 长度不是类型 |
| 容量 | 无 | 有 cap 概念 |

```go
arr := [3]int{1, 2, 3}      // 数组
slice := []int{1, 2, 3}      // 切片
slice = append(slice, 4)     // 可扩容
```

### 9. map 是否是线程安全的？ 🌟（小红书外派、优咔科技）

**答案**：
- **不安全**：并发读写会 panic
- **解决方案**：
  1. 使用 `sync.Mutex` 或 `sync.RWMutex`
  2. 使用 `sync.Map`（适合读多写少）

```go
// 方案1：互斥锁
var mu sync.Mutex
var m = make(map[string]int)

mu.Lock()
m["key"] = 1
mu.Unlock()

// 方案2：sync.Map
var sm sync.Map
sm.Store("key", 1)
```

### 10. sync.Map 的原理？ 🌟（小红书外派、优咔科技）

**答案**：
- 适用于读多写少的场景
- 使用两个 map：`read`（只读）和 `dirty`（读写）
- 读取先查 `read`，未命中再查 `dirty`
- 使用原子操作减少锁竞争

### 11. Go 读写锁的概念？ 🌟（展盟）

**答案**：
- `sync.RWMutex` 支持多个读操作或一个写操作
- 读操作不互斥，写操作互斥
- 读优先还是写优先取决于具体实现

```go
var rw sync.RWMutex

// 读锁
rw.RLock()
defer rw.RUnlock()

// 写锁
rw.Lock()
defer rw.Unlock()
```

### 12. context 的应用场景？ 🌟（展盟）

**答案**：
- 传递请求范围的值（用户ID、认证token）
- 控制子 goroutine 的生命周期（超时、取消）
- 在 HTTP 请求、RPC 调用中传递上下文

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

go func(ctx context.Context) {
    select {
    case <-ctx.Done():
        fmt.Println("Cancelled")
    }
}(ctx)
```

### 13. select 的作用？ 🌟（展盟）

**答案**：
- 用于监听多个 channel 操作
- 随机选择一个就绪的 case 执行
- 支持超时控制和非阻塞操作

```go
select {
case msg := <-ch1:
    fmt.Println(msg)
case ch2 <- "hello":
    fmt.Println("Sent")
case <-time.After(time.Second):
    fmt.Println("Timeout")
}
```

### 14. 如何判断两个 interface{} 相等？ 🌟（优咔科技）

**答案**：
- 两个 interface 可以用 `==` 比较
- 相等条件：
  1. 都是 nil
  2. 类型相同，值相同
- 注意：包含不可比较类型（如 slice）时会 panic

```go
var i1, i2 interface{} = 1, 1
fmt.Println(i1 == i2)  // true
```

### 15. Go map 中删除一个 key 的内存是否会立即释放？ 🌟（优咔科技）

**答案**：
- 不会立即释放
- 删除只是标记为空，内存仍被占用
- GC 后可能才会真正释放
- 大量删除后可以考虑新建 map

### 16. init() 方法的特性？ 🌟（优咔科技）

**答案**：
- 在 `main` 函数之前执行
- 每个包可以有多个 `init` 函数
- 执行顺序不确定
- 无参数无返回值，不能被调用

执行顺序：`import` → `const` → `var` → `init()` → `main()`

### 17. Go 里面的类型断言？ 🌟（优咔科技）

**答案**：
```go
var i interface{} = "hello"

// 安全断言
s, ok := i.(string)
if !ok {
    // 处理类型不匹配
}

// 类型判断
switch v := i.(type) {
case string:
    fmt.Println("string:", v)
case int:
    fmt.Println("int:", v)
}
```

### 18. sync 包使用？ 🌟（优咔科技、360）

**答案**：
主要类型：
- `sync.Mutex` - 互斥锁
- `sync.RWMutex` - 读写锁
- `sync.WaitGroup` - 等待组
- `sync.Once` - 只执行一次
- `sync.Pool` - 对象池
- `sync.Map` - 并发安全的 map

---

## 并发与调度

### 19. GMP 模型？ 🌟（百度、滴滴、小米、展盟）

**答案**：
- **G**（Goroutine）：协程
- **M**（Machine）：线程
- **P**（Processor）：调度器，每个 P 有本地队列

**调度流程**：
1. 创建 G，放入本地队列或全局队列
2. M 从 P 的队列获取 G 执行
3. Work Stealing：队列空时从其他 P 偷取 G
4. Hand Off：G 阻塞时，P 转移给其他 M

### 20. GMP 能不能去掉 P 层？ 🌟（百度）

**答案**：
不能去掉 P 的原因：
- 减少全局队列锁竞争
- 每个 P 有本地队列，提升并发性能
- 支持 Work Stealing 算法
- 更好的负载均衡

### 21. Goroutine 什么情况会发生内存泄漏？ 🌟（展盟、百度、360）

**答案**：
1. **Goroutine 泄漏**：
   - 发送到无接收者的 channel 阻塞
   - 从无发送者的 channel 接收阻塞
   - 死锁

2. **避免方法**：
   - 使用带缓冲的 channel
   - 设置超时（context.WithTimeout）
   - 使用 select + default

### 22. channel 死锁的场景？ 🌟（百度）

**答案**：
1. 无缓冲 channel 无接收者直接发送
2. 有缓冲 channel 满时继续发送
3. 所有 goroutine 都在等待，无 default
4. 多个 goroutine 互相等待

**解决方案**：
- 使用 select + default
- 设置超时
- 使用缓冲 channel

### 23. 对关闭的 channel 进行读写会发生什么？ 🌟（百度）

**答案**：
- **读关闭的 channel**：
  - 有数据：返回数据 + true
  - 无数据：返回零值 + false
  - 不会阻塞，不会 panic

- **写关闭的 channel**：
  - 直接 panic

```go
ch := make(chan int, 1)
ch <- 1
close(ch)

val, ok := <-ch  // 1, true
val, ok = <-ch   // 0, false
ch <- 2          // panic
```

### 24. channel 底层实现？ 🌟（360）

**答案**：
- 数据结构：`hchan`
- 组成部分：
  - `buf`：环形缓冲区
  - `sendx`/`recvx`：发送/接收索引
  - `lock`：互斥锁
  - `sendq`/`recvq`：等待队列
- 线程安全，通过锁保护

### 25. Work Stealing 算法？ 🌟（百度）

**答案**：
- 当 P 的本地队列为空时
- 从其他 P 的本地队列"偷取"一半的 G
- 减少空转，提高资源利用率
- 偷取顺序：全局队列 → 其他 P 本地队列

### 26. 如果有一个 G 一直占用资源怎么办？ 🌟（百度）

**答案**：
Go 调度器会切换到**饥饿模式**：
- 新请求的 G 排队等待
- 禁用自旋
- 保证公平性
- 通过 `runtime.Gosched()` 可主动让出 CPU

---

## 数据结构与集合

### 27. slice 扩容机制？ 🌟（深X服、小红书、字节跳动）

**答案**：
**Go 1.18+**：
- 新容量 ≥ 旧容量 2 倍：直接使用新容量
- 旧容量 < 256：新容量 = 旧容量 × 2
- 旧容量 ≥ 256：新容量 = 旧容量 + (旧容量 + 3×256) / 4

```go
s := make([]int, 0)
s = append(s, 1, 2, 3)  // 触发扩容
```

### 28. map 底层实现？ 🌟（小红书、深X服、字节跳动）

**答案**：
- 哈希表实现
- 核心结构：`hmap` 和 `bucket`
- 每个 bucket 存储 8 个 key-value
- 使用链表解决冲突（溢出桶）
- 哈希函数：支持 AES 优化

### 29. map 如何解决 hash 冲突？ 🌟（小红书）

**答案**：
- 链地址法
- 每个 bucket 有 8 个槽位
- 槽位满时使用溢出桶
- 溢出桶形成链表

### 30. map 为什么是无序的？ 🌟（字节跳动）

**答案**：
1. 哈希值决定存储位置
2. 扩容时重新哈希，顺序改变
3. 遍历随机起始点
4. 强制开发者不依赖顺序

**有序遍历方法**：
```go
keys := make([]string, 0, len(m))
for k := range m {
    keys = append(keys, k)
}
sort.Strings(keys)
for _, k := range keys {
    fmt.Println(k, m[k])
}
```

### 31. map 如何判断包含某个 key？ 🌟（字节跳动）

**答案**：
```go
value, ok := m["key"]
if ok {
    // key 存在
    fmt.Println(value)
}
```

---

## 内存管理与GC

### 32. Go 的垃圾回收机制？ 🌟（百度）

**答案**：
**演进历程**：
- Go 1.3：标记清除
- Go 1.5：三色标记
- Go 1.8+：三色标记 + 混合写屏障

**三色标记**：
- 白色：未标记（待回收）
- 灰色：已标记，子对象待处理
- 黑色：已标记，子对象已处理

**混合写屏障**：
- GC 开始时栈上对象全黑
- 新对象黑色
- 被删除引用的对象标记灰色
- 被添加引用的对象标记灰色

### 33. 逃逸分析？ 🌟（百度）

**答案**：
- 编译时分析变量生命周期
- 决定分配在栈还是堆
- 栈分配：快，自动释放
- 堆分配：慢，GC 回收

**查看逃逸**：
```bash
go build -gcflags="-m -l" main.go
```

### 34. Go 内存管理原理？ 🌟（百度、深X服） 🌟（百度）

**答案**：
参考 tcmalloc：
- **mheap**：内存池，管理大块内存
- **mcentral**：按 sizeclass 组织 span
- **mcache**：每个 P 的本地缓存

**内存单位**：
- Page：8KB
- Span：连续 Page
- Object：实际存储单元

### 35. 如何判断对象分配在栈还是堆？ 🌟（百度、字节跳动）

**答案**：
- 自动逃逸分析
- **栈分配条件**：
  - 局部变量
  - 不被外部引用
  - 大小合理

- **堆分配条件**：
  - 被外部引用
  - 变量大小不确定
  - 逃逸分析确定

### 36. Go 内存泄漏常见原因？

**答案**：
1. Goroutine 泄漏
2. time.Ticker 未关闭
3. Finalizer 使用不当
4. 切片/字符串引用大数组
5. 循环引用

---

## 数据库与缓存

### 37. MySQL 事务隔离级别？ 🌟（字节跳动） 🌟（展盟、360）

**答案**：
1. **RU**（Read Uncommitted）：读未提交
2. **RC**（Read Committed）：读已提交
3. **RR**（Repeatable Read）：可重复读（默认）
4. **Serializable**：串行化

### 38. MySQL 可重复读怎么实现？

**答案**：
- 通过 **MVCC**（多版本并发控制）
- Read View 保存事务开始时的快照
- 读取可见版本的数据
- 避免脏读、不可重复读

### 39. 幻读怎么解决？ 🌟（字节跳动）

**答案**：
- Next-Key Lock（记录锁 + 间隙锁）
- RR 隔离级别下自动解决
- RC 隔离级别仍可能存在

### 40. MySQL 联合索引最左匹配原则？ 🌟（字节跳动） 🌟（展盟）

**答案**：
- 联合索引 (a, b, c)
- 以下情况能使用索引：
  - a
  - a, b
  - a, b, c
- 以下情况不能：
  - b
  - b, c
  - c

### 41. MySQL 慢查询优化？ 🌟（字节跳动） 🌟（展盟、360、小米）

**答案**：
1. 开启慢查询日志
2. 使用 EXPLAIN 分析
3. 优化索引
4. 避免 SELECT *
5. 分页优化（延迟关联）
6. 避免深分页

### 42. Redis 数据类型？ 🌟（优咔科技、360）

**答案**：
- **String**：字符串
- **Hash**：哈希表
- **List**：列表
- **Set**：集合
- **ZSet**：有序集合
- **Bitmap**：位图
- **HyperLogLog**：基数统计

### 43. Redis 持久化方式？ 🌟（优咔科技） 🌟（优咔科技）

**答案**：
1. **RDB**（快照）：
   - fork 子进程
   - 定时保存
   - 文件小，恢复快

2. **AOF**（日志）：
   - 记录写命令
   - 数据更完整
   - 文件大，恢复慢

3. **混合持久化**：
   - RDB + AOF
   - 结合两者优势

### 44. Redis ZSet 底层实现？ 🌟（360、深X服、字节跳动） 🌟（360、深X服） 🌟（360）

**答案**：
- **压缩链表**（元素少时）
- **跳表**（元素多时）
- 结合字典实现 O(1) 查找

---

## 网络与框架

### 45. Gin 框架的特点？ 🌟（优咔科技）

**答案**：
- 基于 httprouter
- 高性能
- 中间件机制
- JSON 验证
- 路由分组
- 错误管理

### 46. Gin 并发处理？

**答案**：
- 每个 HTTP 请求一个 goroutine
- 并发安全，context 隔离
- 使用 `sync.Pool` 优化内存

### 47. 如何实现优雅关闭？

**答案**：
```go
srv := &http.Server{Addr: ":8080"}

go func() {
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatal(err)
    }
}()

// 监听信号
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit

// 优雅关闭
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := srv.Shutdown(ctx); err != nil {
    log.Fatal(err)
}
```

---

## 算法题

### 48. 两个 goroutine 交替打印字母和数字 🌟（百度、滴滴、字节跳动）

**答案**：
```go
func main() {
    ch1 := make(chan bool)
    ch2 := make(chan bool)
    
    go func() {
        for i := 0; i < 10; i++ {
            <-ch1
            fmt.Printf("%c ", 'a'+i)
            ch2 <- true
        }
    }()
    
    go func() {
        for i := 1; i <= 10; i++ {
            <-ch2
            fmt.Printf("%d ", i)
            ch1 <- true
        }
    }()
    
    ch1 <- true
    time.Sleep(time.Second)
}
// 输出：a 1 b 2 c 3 ...
```

### 49. 使用 context 取消多个 goroutine

**答案**：
```go
func worker(ctx context.Context, id int) {
    for {
        select {
        case <-ctx.Done():
            fmt.Printf("Worker %d cancelled\n", id)
            return
        default:
            fmt.Printf("Worker %d working\n", id)
            time.Sleep(500 * time.Millisecond)
        }
    }
}

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    
    for i := 1; i <= 3; i++ {
        go worker(ctx, i)
    }
    
    time.Sleep(3 * time.Second)
}
```

---

## 项目与职业规划

### 50. 项目中遇到过什么难点？ 🌟（小红书、百度、滴滴、360、小米）

**答案**：
选择 1-2 个有代表性的技术难点，按照 STAR 原则描述：
- **Situation**：背景
- **Task**：任务
- **Action**：行动
- **Result**：结果

### 51. 职业规划？ 🌟（展盟）

**答案**：
- **短期**（1-2年）：深入技术栈，承担核心开发
- **中期**（3-5年）：技术专家或架构方向
- **长期**（5年以上）：技术管理或专业深耕

---

## 面试建议

1. **不要死记硬背**：结合项目经验
2. **主动引导**：展示擅长的领域
3. **准备项目**：2-3 个有深度的功能模块
4. **算法练习**：LeetCode 中等难度
5. **源码阅读**：Go 标准库或常用框架

---

## 参考资料

- [Go 官方文档](https://go.dev/doc/)
- [Go 语言圣经](https://books.studygolang.com/gopl-zh/)
- [Draveness Go 系列](https://draveness.me/golang/)

---

*文档整理自小红书及其他平台 Go 后端面试经验*