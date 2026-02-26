# 消息翻译功能设计

**日期**: 2026-02-26
**功能**: 消息翻译 (Message Translation)

---

## 1. 功能概述

为 KamaChat IM 系统增加消息翻译功能，用户可以手动触发将聊天消息翻译成英语。

---

## 2. 技术方案

| 项目 | 选型 |
|------|------|
| AI 模型 | Kimi K2.5 (Moonshot AI) |
| API 平台 | ModelScope 魔搭社区 |
| API 地址 | `https://api-inference.modelscope.cn/v1/` |
| SDK | OpenAI Go SDK |
| 翻译方向 | 原文 → 英语 |

---

## 3. 功能流程

```
用户点击翻译按钮
    ↓
前端 POST /api/message/translate
    参数: { messageId, targetLang: "en" }
    ↓
后端根据 messageId 查询原消息内容
    ↓
调用 K2.5 翻译 API
    ↓
返回翻译结果给前端
    ↓
前端展示翻译文本
```

---

## 4. API 设计

### 请求

```
POST /api/message/translate
Authorization: Bearer <token>
Content-Type: application/json

{
    "messageId": "消息UUID",
    "targetLang": "en"
}
```

### 响应

```json
{
    "code": 0,
    "msg": "success",
    "data": {
        "originalText": "原文内容",
        "translatedText": "翻译结果"
    }
}
```

---

## 5. 目录结构

```
internal/
├── dto/
│   ├── request/
│   │   └── message/
│   │       └── translate_request.go    # 翻译请求 DTO
│   └── respond/
│       └── message/
│           └── translate_respond.go    # 翻译响应 DTO
├── service/
│   └── translation/
│       └── service.go                  # 翻译服务
└── router/
    └── translate_routes.go             # 翻译路由
```

---

## 6. 配置新增

```toml
# configs/config.toml
[modelScopeConfig]
apiKey = "你的ModelScope Token"
baseUrl = "https://api-inference.modelscope.cn/v1"
model = "moonshotai/Kimi-K2.5"
```

---

## 7. Prompt 设计

```
你是一个专业的翻译助手。请将下面的消息翻译成英语，只返回翻译结果，不要有任何解释：

{消息内容}
```

---

## 8. 错误处理

| 场景 | HTTP 状态码 | 错误码 | 说明 |
|------|------------|--------|------|
| messageId 不存在 | 400 | -2 | 消息不存在 |
| 消息内容为空 | 400 | -2 | 空消息无法翻译 |
| API 调用失败 | 500 | -1 | 翻译服务异常 |

---

## 9. 待定事项

- [ ] 确认 ModelScope Token 格式
- [ ] 确认 K2.5 模型具体名称



