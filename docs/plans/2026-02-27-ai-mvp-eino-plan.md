# KamaChat AI MVP 方案（Eino 版）

> 日期：2026-02-27
> 
> 范围确认：**不做语音转文字**，仅做：
> 1) 智能回复建议
> 2) 群聊总结
> 3) 多语言聊天翻译

---

## 1. 目标与原则

### 1.1 目标
在仿微信聊天场景中，用最小改动落地 3 个高频 AI 功能，优先提升聊天效率与群聊管理体验。

### 1.2 设计原则
- AI 是“辅助”，不自动替用户发送消息。
- 先走 HTTP API，同步返回；群聊总结支持异步任务扩展。
- 保持现有分层：`router -> handler -> service -> dao`。
- 基于 **ByteDance Eino** 统一编排 Prompt、模型调用、容错与可观测。

---

## 2. 功能范围（MVP）

### 2.1 智能回复建议（P0）
**场景**：用户在单聊/群聊输入框中点击“AI 建议”，返回 3 条可选回复。

**输入**：
- 会话 ID（或 target_id）
- 最近 N 条消息（可由后端自动拉取）
- 用户草稿（可选）
- 风格（简洁/礼貌/商务）

**输出**：
- 3 条候选回复（长度控制 + 风格一致）

### 2.2 群聊总结（P0）
**场景**：在群聊中生成“本日要点 + 待办 + 决策”。

**输入**：
- group_id
- 时间范围（如最近 24h）
- 消息条数上限（如 200）

**输出**：
- 摘要文本
- TODO 列表（含负责人名，若可识别）
- 关键决策点

### 2.3 多语言聊天翻译（P1）
**场景**：用户对某条消息执行翻译，或设置自动翻译目标语言。

**输入**：
- message_uuid 或原文 text
- source_lang（可空，自动识别）
- target_lang（必填，如 `zh-CN` / `en` / `ja`）

**输出**：
- 原语言识别结果
- 翻译文本

---

## 3. API 设计（对齐现有风格）

建议新增路由文件：`internal/router/ai_routes.go`，挂在私有路由组（JWT 后）。

### 3.1 智能回复建议
`POST /ai/reply-suggestions`

请求体：
```json
{
  "target_id": "U123456",
  "style": "polite",
  "draft": "我晚点回复你",
  "context_limit": 20
}
```

响应体：
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "suggestions": [
      "收到，我晚些时候详细回复你。",
      "我先看下，稍后给你完整反馈。",
      "这会儿不太方便，晚一点我回你。"
    ]
  }
}
```

### 3.2 群聊总结
`POST /ai/group-summary`

请求体：
```json
{
  "group_id": "G10001",
  "hours": 24,
  "limit": 200,
  "style": "brief"
}
```

响应体：
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "summary": "今日讨论主要围绕版本发版时间与风险回归...",
    "todos": [
      "张三：周五前补齐压测报告",
      "李四：确认灰度发布窗口"
    ],
    "decisions": [
      "本周五 20:00 开始灰度",
      "出现 P1 故障立即回滚"
    ]
  }
}
```

### 3.3 多语言翻译
`POST /ai/translate`

请求体：
```json
{
  "text": "Can we move the release to next Monday?",
  "source_lang": "",
  "target_lang": "zh-CN"
}
```

响应体：
```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "detected_lang": "en",
    "translated_text": "我们可以把发布时间改到下周一吗？"
  }
}
```

---

## 4. 目录与代码落点

### 4.1 建议新增目录
- `internal/handler/ai_handler.go`
- `internal/router/ai_routes.go`
- `internal/service/ai/service.go`
- `internal/service/ai/prompt.go`
- `internal/service/ai/types.go`
- `internal/dto/request/ai/*.go`
- `internal/dto/respond/ai/*.go`
- `internal/infrastructure/ai/eino_client.go`
- `internal/infrastructure/ai/eino_flows.go`

### 4.2 依赖注入改造点
- `internal/service/interfaces.go`：新增 `AIService` 接口。
- `internal/service/provider.go`：`Services` 增加 `AI AIService`。
- `internal/handler/provider.go`：`Handlers` 增加 `AI *AIHandler`。
- `internal/router/router.go`：注册 `RegisterAIRoutes(private)`。

---

## 5. Eino 编排设计

> 说明：下面是逻辑编排，不绑定具体模型厂商；可继续使用你现有配置中的 `modelScopeConfig`，也可升级为 `aiConfig`。

### 5.1 智能回复建议 Flow
1. 加载会话上下文（最近 N 条消息）
2. 构建 Prompt（角色 + 语气 + 长度约束）
3. Eino ChatModel 调用
4. 结构化解析为 `[]string`（最多 3 条）
5. 内容安全过滤后返回

### 5.2 群聊总结 Flow（建议 Map-Reduce）
1. 拉取群消息并分片（按 token 预算）
2. `Map`：每片生成子摘要
3. `Reduce`：合并为最终摘要 + TODO + 决策
4. 结果写缓存（Redis，短期）
5. 返回结构化结果

### 5.3 翻译 Flow
1. 语言识别（若 `source_lang` 为空）
2. 翻译到 `target_lang`
3. 术语保护（人名/群名/专有词不误译）
4. 返回结果，并对热门文本做缓存

---

## 6. 数据模型建议

MVP 尽量轻量，建议先加两张表。

### 6.1 `ai_request_log`（审计与成本统计）
- `id` bigint pk
- `request_id` varchar(64)
- `user_id` varchar(64)
- `scene` varchar(32)  // reply_suggestion/group_summary/translate
- `target_id` varchar(64)
- `model_name` varchar(128)
- `prompt_tokens` int
- `completion_tokens` int
- `status` tinyint      // 0失败 1成功
- `err_msg` varchar(255)
- `latency_ms` int
- `created_at` datetime

### 6.2 `ai_summary_snapshot`（群聊总结留存，可选）
- `id` bigint pk
- `group_id` varchar(64)
- `operator_id` varchar(64)
- `time_range_start` datetime
- `time_range_end` datetime
- `summary_text` text
- `todos_json` json
- `decisions_json` json
- `created_at` datetime

---

## 7. Redis Key 设计

- `ai:reply:{user_id}:{target_id}:{hash}`（TTL 60s）
- `ai:translate:{target_lang}:{hash(text)}`（TTL 24h）
- `ai:summary:{group_id}:{time_window}`（TTL 10min）
- `ai:ratelimit:{user_id}:{scene}:{yyyyMMddHH}`

---

## 8. 配置设计（ModelScope + Kimi K2.5）

你当前项目已存在 `modelScopeConfig`，本期直接沿用即可：

```toml
[modelScopeConfig]
apiKey = "" # 不建议写死，使用环境变量注入
baseUrl = "https://api-inference.modelscope.cn/v1"
model = "moonshotai/Kimi-K2.5"
```

环境变量（推荐）：
- `MODELSCOPE_API_KEY`（优先）
- `MODELSCOPE_BASE_URL`
- `MODELSCOPE_MODEL`

兼容变量（已支持）：
- `AI_API_KEY`
- `AI_BASE_URL`
- `AI_DEFAULT_MODEL`

Windows PowerShell 示例：
```powershell
$env:MODELSCOPE_API_KEY="ms-你的Key"
$env:MODELSCOPE_MODEL="moonshotai/Kimi-K2.5"
go run cmd/kama_chat_server/main.go
```

---

## 9. 鉴权与安全

- 所有 `/ai/*` 路由走 JWT。
- 群聊总结前必须校验操作者是群成员。
- 翻译/建议请求写审计日志（可排查滥用）。
- 加超时、重试、熔断和降级文案（例如“AI 服务繁忙，请稍后再试”）。
- 私聊内容默认不落长期存储，仅保留必要调用日志字段。

---

## 10. 迭代顺序（建议 2 周）

### Week 1
1. 接入 Eino 基础能力（client + model + flow）
2. 打通 `/ai/translate`
3. 打通 `/ai/reply-suggestions`

### Week 2
1. 打通 `/ai/group-summary`
2. 加缓存 + 限流 + 审计日志
3. 压测与成本观测（P95 时延、token 消耗）

---

## 11. 验收标准（MVP）

- 智能回复：返回 3 条候选，P95 < 2s。
- 群聊总结：200 条消息以内，P95 < 5s。
- 翻译：中英互译准确可读，P95 < 1.5s。
- 三个接口都有限流、超时、错误码和日志。

---

## 12. 不在本期范围

- 语音转文字（按需求明确排除）
- AI 自动代发消息
- 知识库 RAG 与企业文档问答

---

如果要继续实现代码，建议从 `translate` 先落地（链路最短、收益最快），再复用同一套 Eino client 到“智能回复”和“群聊总结”。
