# KamaChat Web 前端设计方案

**创建日期**: 2026-02-25
**项目**: KamaChat Web 前端
**技术栈**: Vue 3 + Vite + Element Plus + Pinia + TypeScript

---

## 1. 项目概述

### 1.1 目标
基于 KamaChat Go 后端，创建 Vue 3 Web 前端，实现完整的即时通讯功能。

### 1.2 技术选型

| 技术 | 版本 | 用途 |
|------|------|------|
| Vue 3 | ^3.4 | 核心框架 |
| Vite | ^5.0 | 构建工具 |
| Element Plus | ^2.5 | UI 组件库 |
| Pinia | ^2.1 | 状态管理 |
| TypeScript | ^5.0 | 类型支持 |
| Axios | ^1.6 | HTTP 请求 |
| Vue Router | ^4.2 | 路由管理 |



---

## 2. 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                      Vue 3 Web 前端                         │
├─────────────────────────────────────────────────────────────┤
│  视图层 (Vue 3 Composition API)                             │
├─────────────────────────────────────────────────────────────┤
│  状态管理层 (Pinia)                                         │
│    - userStore (用户状态)                                   │
│    - chatStore (会话/消息状态)                              │
│    - contactStore (好友/群组状态)                           │
├─────────────────────────────────────────────────────────────┤
│  网络层 (Axios + WebSocket)                                │
├─────────────────────────────────────────────────────────────┤
│  UI 组件库 (Element Plus)                                  │
└─────────────────────────────────────────────────────────────┘
```

---

## 3. 页面结构

| 路由 | 页面 | 功能 |
|------|------|------|
| `/login` | 登录页 | 账号密码登录 |
| `/register` | 注册页 | 用户注册 |
| `/sms-login` | 短信登录 | 手机验证码登录 |
| `/home` | 主页面 | 底部 Tab 导航 |
| `/home/conversations` | 会话列表 | 聊天会话管理 |
| `/home/friends` | 好友列表 | 好友管理 |
| `/home/groups` | 群组列表 | 群组管理 |
| `/home/profile` | 个人中心 | 个人信息设置 |

---

## 4. 功能模块

### 4.1 认证模块

| 功能 | 后端 API |
|------|----------|
| 账号密码登录 | `POST /api/auth/login` |
| 短信验证码登录 | `POST /api/auth/sms-login` |
| 发送验证码 | `POST /api/auth/sms-code` |
| 用户注册 | `POST /api/auth/register` |
| 刷新 Token | `POST /api/auth/refresh` |

### 4.2 用户模块

| 功能 | 后端 API |
|------|----------|
| 获取个人信息 | `GET /api/user/info` |
| 更新用户信息 | `PUT /api/user/info` |
| 获取他人公开信息 | `GET /api/user/public-info` |
| 上传头像 | `POST /api/upload/avatar` |

### 4.3 好友模块

| 功能 | 后端 API |
|------|----------|
| 获取好友列表 | `GET /api/friends` |
| 获取好友详情 | `GET /api/friends/info` |
| 添加好友 | `POST /api/friends/apply` |
| 删除好友 | `DELETE /api/friends` |
| 拉黑好友 | `POST /api/friends/block` |
| 取消拉黑 | `DELETE /api/friends/block` |
| 更新好友备注 | `PUT /api/friends/remark` |
| 获取好友申请列表 | `GET /api/friends/applies` |
| 通过/拒绝好友申请 | `POST /api/friends/applies/approve` |

### 4.4 群组模块

| 功能 | 后端 API |
|------|----------|
| 创建群组 | `POST /api/groups` |
| 获取我创建的群组 | `GET /api/groups/owned` |
| 获取已加入的群组 | `GET /api/groups/joined` |
| 获取群详情 | `GET /api/groups/detail` |
| 更新群信息 | `PUT /api/groups/info` |
| 解散群组 | `DELETE /api/groups` |
| 退出群组 | `POST /api/groups/leave` |
| 获取群成员列表 | `GET /api/groups/members` |
| 移除群成员 | `DELETE /api/groups/members` |
| 禁言/取消禁言 | `POST /api/groups/members/mute` |
| 申请加群 | `POST /api/groups/apply` |
| 获取入群申请列表 | `GET /api/groups/applies` |
| 通过/拒绝入群申请 | `POST /api/groups/applies/approve` |

### 4.5 会话模块

| 功能 | 后端 API |
|------|----------|
| 检查是否允许打开会话 | `GET /api/sessions/check` |
| 打开/创建会话 | `POST /api/sessions` |
| 获取单聊会话列表 | `GET /api/sessions/direct` |
| 获取群聊会话列表 | `GET /api/sessions/group` |
| 删除会话 | `DELETE /api/sessions` |
| 置顶/取消置顶 | `PUT /api/sessions/pin` |

### 4.6 消息模块

| 功能 | 后端 API |
|------|----------|
| 获取消息列表 | `GET /api/messages` |
| 撤回消息 | `POST /api/messages/recall` |
| 上传聊天文件 | `POST /api/upload/file` |

### 4.7 WebSocket 实时通信

- **连接地址**: `ws://{host}/ws`
- **认证**: 通过 URL 参数或握手协议传递 Token
- **消息类型**: 私聊消息、群聊消息、好友请求、系统通知
- **心跳机制**: 定时发送 ping/pong 保持连接

---

## 5. 项目结构

```
chat-web/
├── public/
├── src/
│   ├── api/                 # API 封装
│   │   ├── auth.ts
│   │   ├── user.ts
│   │   ├── friends.ts
│   │   ├── groups.ts
│   │   ├── sessions.ts
│   │   └── messages.ts
│   ├── assets/              # 静态资源
│   ├── components/          # 公共组件
│   │   ├── ChatInput.vue
│   │   ├── MessageList.vue
│   │   ├── MessageBubble.vue
│   │   ├── SessionItem.vue
│   │   ├── ContactItem.vue
│   │   └── Avatar.vue
│   ├── composables/         # 组合式函数
│   │   ├── useWebSocket.ts
│   │   ├── useAuth.ts
│   │   └── useChat.ts
│   ├── layouts/             # 布局组件
│   │   └── MainLayout.vue
│   ├── router/              # 路由配置
│   │   └── index.ts
│   ├── stores/              # Pinia 状态管理
│   │   ├── user.ts
│   │   ├── chat.ts
│   │   └── contact.ts
│   ├── styles/              # 全局样式
│   │   └── index.scss
│   ├── types/               # TypeScript 类型
│   │   └── index.ts
│   ├── utils/               # 工具函数
│   │   ├── request.ts       # Axios 封装
│   │   └── storage.ts       # 本地存储
│   ├── views/               # 页面视图
│   │   ├── Login.vue
│   │   ├── Register.vue
│   │   ├── SmsLogin.vue
│   │   ├── Home.vue
│   │   ├── conversations/
│   │   │   └── index.vue
│   │   ├── friends/
│   │   │   └── index.vue
│   │   ├── groups/
│   │   │   └── index.vue
│   │   └── profile/
│   │       └── index.vue
│   ├── App.vue
│   └── main.ts
├── index.html
├── package.json
├── vite.config.ts
├── tsconfig.json
└── .env
```

---

## 6. 关键设计决策

### 6.1 Token 存储策略
- **accessToken**: 存 localStorage（用于 API 请求）
- **refreshToken**: 存 Cookie（HttpOnly，HTTP Only）

### 6.2 WebSocket 重连
- 断线自动重连（指数退避策略）
- 重连前检查网络状态

### 6.3 消息列表分页
- 游标分页（cursor-based pagination）
- 向上滚动加载历史消息
- 每次加载 20 条

### 6.4 状态管理
- **userStore**: 用户信息、登录状态、Token
- **chatStore**: 当前会话、消息列表、WS 连接状态
- **contactStore**: 好友列表、群组列表、申请列表

---

## 7. UI 设计风格

- **风格**: 简约现代
- **主色调**: #409EFF (Element Plus 蓝色)
- **布局**: 左侧会话列表 + 右侧聊天窗口（类似微信桌面版）
- **移动端**: 底部 Tab 切换（会话/好友/群组/我的）

---

## 8. 验收标准

- [ ] 用户可以登录、注册、短信登录
- [ ] 可以查看会话列表，打开聊天窗口
- [ ] 可以发送和接收消息（文字、图片、文件）
- [ ] 可以管理好友（添加、删除、拉黑、备注）
- [ ] 可以管理群组（创建、加入、退群、群管理）
- [ ] 消息撤回功能正常
- [ ] WebSocket 实时推送正常
- [ ] 响应式布局适配移动端
