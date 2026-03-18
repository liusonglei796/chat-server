# 工作描述

参与开发 KamaChat 项目，一个基于 Go 的即时通讯服务端，支持单聊群聊、WebSocket 实时消息推送、好友与群组管理、消息撤回、会话置顶、文件上传、短信验证码登录、音视频信令转发等功能。项目采用 gin、gorm、gorilla/websocket、redis、kafka、jwt、zap、snowflake、docker 等技术栈。

# 主要工作

- 基于 websocket 搭建聊天服务器，每个在线用户维护独立的 Read/Write 双 goroutine，处理单聊群聊消息的实时收发、登录登出。
- 使用 kafka 作为消息总线，ws 网关收到消息后发布到 kafka，消费者按消息类型（文本/文件/音视频信令）分发到目标用户的 channel，实现消息收发解耦和流量削峰。
- 基于 gorm 搭建数据访问层，采用 Repository 模式封装 CRUD，使用 redis 做缓存降低数据库查询压力。
- 封装 `cache.Helper` 组件实现 Cache-Aside 模式，集成 singleflight 防缓存击穿、空值缓存防穿透、TTL 随机抖动防雪崩。
- 实现 JWT 双 Token 认证（Access Token + Refresh Token），Refresh Token 通过 tokenID 存 Redis 实现单点登录互踢。
- 实现基于 Redis 的固定窗口限流中间件，支持按 IP、手机号等维度限流，Redis 故障时降级放行。
- 集成阿里云短信服务实现验证码登录，包含频率限制、验证码缓存预占和发送失败回滚机制。
- 支持 HTTPS 部署，生产环境由 Nginx 做 SSL 终止后反向代理到 Go 服务，避免应用层重复加解密。
- 使用 snowflake 算法生成全局唯一消息 ID，输出 string 类型避免前端 JS 精度丢失。
- 使用 go-wrk 完成多场景压测（50/200/500 并发），单机稳定在 800-1000 RPS，P99 < 310ms。

# 项目难点

- 高并发下的长连接管理：用户断线不通知会产生僵尸连接，通过 Ping/Pong 心跳检测 + sync.Once 保证 cleanup 幂等执行；封装 safeClose 防止 double-close channel panic；trySendBack 非阻塞写入防止慢客户端阻塞 Kafka 消费者。
- 缓存三大问题防护：穿透用空值缓存拦截、击穿用 singleflight 合并并发回源、雪崩用 TTL 随机抖动打散过期时间，统一封装到 cache.Helper 组件中复用。
- 消息解耦与流量削峰：引入 Kafka 消息队列，解决高峰期消息堆积问题，实现 WebSocket 网关与业务处理的解耦，提升系统稳定性。

# 个人收获

- 技术深度提升：理解了 Go 并发模型（goroutine + channel）在 WebSocket 场景的实际应用，掌握了 sync.Map、sync.Once、singleflight 等并发原语的使用场景；熟练使用 Kafka 做消息解耦，理解 Producer/Consumer 模式和消费组机制。
- 系统设计能力：掌握了 Cache-Aside 缓存策略的完整实现，理解穿透/击穿/雪崩的本质和对应方案；实现 JWT 双 Token 认证流程，理解 Access/Refresh 分离的设计意图。
- 问题排查能力：学会使用 go-wrk 压测和 pprof 火焰图定位性能瓶颈，形成压测→分析→优化→复测的排查流程。
- 工程化实践：落地了统一错误码（errorx）、结构化日志（Zap）、Docker Compose 部署等规范，对代码可维护性有了更深认识。
