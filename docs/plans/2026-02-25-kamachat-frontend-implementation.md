# KamaChat Web 前端实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 创建 Vue 3 + Element Plus Web 前端，完整对接 KamaChat 后端，实现即时通讯功能

**Architecture:** 使用 Vue 3 Composition API + Pinia 状态管理 + Element Plus UI 组件，WebSocket 实现实时消息推送

**Tech Stack:** Vue 3.4 + Vite 5 + Element Plus 2.5 + Pinia 2.1 + TypeScript 5 + Axios 1.6

---

## 前置准备

### Task 1: 创建项目目录结构

**Files:**
- Create: `chat-web/package.json`
- Create: `chat-web/vite.config.ts`
- Create: `chat-web/tsconfig.json`
- Create: `chat-web/index.html`
- Create: `chat-web/.env`

**Step 1: 创建 package.json**

```json
{
  "name": "kamachat-web",
  "version": "1.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "vue": "^3.4.0",
    "vue-router": "^4.2.0",
    "pinia": "^2.1.0",
    "element-plus": "^2.5.0",
    "axios": "^1.6.0",
    "@element-plus/icons-vue": "^2.3.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.0.0",
    "typescript": "^5.0.0",
    "vite": "^5.0.0",
    "vue-tsc": "^1.8.0",
    "sass": "^1.69.0"
  }
}
```

**Step 2: 创建 vite.config.ts**

```typescript
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8000',
        changeOrigin: true
      },
      '/ws': {
        target: 'ws://localhost:8000',
        ws: true
      }
    }
  }
})
```

**Step 3: 创建 tsconfig.json**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "module": "ESNext",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "preserve",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"]
    }
  },
  "include": ["src/**/*.ts", "src/**/*.tsx", "src/**/*.vue"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

**Step 4: 创建 index.html**

```html
<!DOCTYPE html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <link rel="icon" type="image/svg+xml" href="/vite.svg" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>KamaChat</title>
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
```

**Step 5: 创建 .env**

```
VITE_APP_TITLE=KamaChat
VITE_APP_API_BASE=/api
VITE_APP_WS_URL=ws://localhost:8000/ws
```

---

### Task 2: 创建入口文件

**Files:**
- Create: `chat-web/src/main.ts`
- Create: `chat-web/src/App.vue`
- Create: `chat-web/src/styles/index.scss`

**Step 1: 创建 main.ts**

```typescript
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import App from './App.vue'
import router from './router'
import './styles/index.scss'

const app = createApp(App)

// 注册所有图标
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(createPinia())
app.use(router)
app.use(ElementPlus)

app.mount('#app')
```

**Step 2: 创建 App.vue**

```vue
<template>
  <RouterView />
</template>

<script setup lang="ts">
import { RouterView } from 'vue-router'
</script>

<style>
#app {
  width: 100%;
  height: 100vh;
}
</style>
```

**Step 3: 创建全局样式**

```scss
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

html, body {
  width: 100%;
  height: 100%;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
}

.page-container {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}
```

---

### Task 3: 创建路由配置

**Files:**
- Create: `chat-web/src/router/index.ts`
- Create: `chat-web/src/router/routes.ts`

**Step 1: 创建路由定义**

```typescript
import type { RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/login'
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue')
  },
  {
    path: '/register',
    name: 'Register',
    component: () => import('@/views/Register.vue')
  },
  {
    path: '/sms-login',
    name: 'SmsLogin',
    component: () => import('@/views/SmsLogin.vue')
  },
  {
    path: '/home',
    component: () => import('@/views/Home.vue'),
    children: [
      {
        path: '',
        redirect: '/home/conversations'
      },
      {
        path: 'conversations',
        name: 'Conversations',
        component: () => import('@/views/conversations/index.vue')
      },
      {
        path: 'friends',
        name: 'Friends',
        component: () => import('@/views/friends/index.vue')
      },
      {
        path: 'groups',
        name: 'Groups',
        component: () => import('@/views/groups/index.vue')
      },
      {
        path: 'profile',
        name: 'Profile',
        component: () => import('@/views/profile/index.vue')
      }
    ]
  }
]

export default routes
```

**Step 2: 创建 router/index.ts**

```typescript
import { createRouter, createWebHistory } from 'vue-router'
import routes from './routes'
import { useUserStore } from '@/stores/user'

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to, from, next) => {
  const userStore = useUserStore()
  const isAuthenticated = userStore.isAuthenticated
  
  if (to.path !== '/login' && to.path !== '/register' && to.path !== '/sms-login' && !isAuthenticated) {
    next('/login')
  } else if ((to.path === '/login' || to.path === '/register') && isAuthenticated) {
    next('/home')
  } else {
    next()
  }
})

export default router
```

---

### Task 4: 创建类型定义

**Files:**
- Create: `chat-web/src/types/index.ts`

**Step 1: 创建类型定义**

```typescript
// 用户相关
export interface User {
  userId: number
  nickname: string
  avatar: string
  phone: string
  email: string
  birthday: string
  gender: number
  status: number
}

export interface LoginParams {
  phone: string
  password: string
}

export interface SmsLoginParams {
  phone: string
  code: string
}

export interface RegisterParams {
  phone: string
  password: string
  nickname: string
  code: string
}

// 好友相关
export interface Friend {
  friendId: number
  userId: number
  remark: string
  status: number
  user: User
}

export interface FriendApply {
  applyId: number
  fromUserId: number
  toUserId: number
  status: number
  remark: string
  createTime: string
  fromUser: User
}

// 群组相关
export interface Group {
  groupId: number
  groupName: string
  avatar: string
  ownerUserId: number
  memberCount: number
  status: number
}

export interface GroupMember {
  userId: number
  groupId: number
  role: number
  muteUntil: string
  joinTime: string
  user: User
}

export interface GroupApply {
  applyId: number
  groupId: number
  userId: number
  status: number
  remark: string
  createTime: string
  user: User
}

// 会话相关
export interface Session {
  sessionId: number
  sessionType: number // 1: 私聊 2: 群聊
  targetId: number // 好友ID或群组ID
  isPinned: number
  lastMessage: Message | null
  unreadCount: number
  createTime: string
  // 扩展显示用
  name?: string
  avatar?: string
}

// 消息相关
export interface Message {
  messageId: number
  sessionId: number
  senderId: number
  messageType: number // 1: 文本 2: 图片 3: 文件 4: 语音
  content: string
  url: string
  extra: string
  isRecall: number
  createTime: string
}

// API 响应
export interface ApiResponse<T = any> {
  ret: number
  data: T
  msg: string
}

export interface CursorResult<T> {
  list: T[]
  nextCursor: string
  isEnd: boolean
}
```

---

### Task 5: 创建工具函数

**Files:**
- Create: `chat-web/src/utils/request.ts`
- Create: `chat-web/src/utils/storage.ts`
- Create: `chat-web/src/utils/storage.ts`

**Step 1: 创建 axios 封装**

```typescript
import axios, { AxiosInstance, AxiosRequestConfig, AxiosResponse } from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'
import { getToken } from './storage'

const service: AxiosInstance = axios.create({
  baseURL: import.meta.env.VITE_APP_API_BASE,
  timeout: 10000
})

service.interceptors.request.use(
  (config) => {
    const token = getToken()
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

service.interceptors.response.use(
  (response: AxiosResponse) => {
    const res = response.data
    if (res.ret === 0) {
      return res.data
    } else if (res.ret === -2) {
      ElMessage.error(res.msg || '请求失败')
      return Promise.reject(new Error(res.msg || '请求失败'))
    } else if (res.ret === 401) {
      ElMessage.error('登录已过期，请重新登录')
      router.push('/login')
      return Promise.reject(new Error('未授权'))
    } else {
      ElMessage.error(res.msg || '系统错误')
      return Promise.reject(new Error(res.msg || '系统错误'))
    }
  },
  (error) => {
    ElMessage.error(error.message || '网络错误')
    return Promise.reject(error)
  }
)

export default service
```

**Step 2: 创建存储工具**

```typescript
const TOKEN_KEY = 'access_token'
const USER_KEY = 'user_info'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function removeToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

export function getUserInfo<T>(): T | null {
  const user = localStorage.getItem(USER_KEY)
  return user ? JSON.parse(user) : null
}

export function setUserInfo<T>(user: T): void {
  localStorage.setItem(USER_KEY, JSON.stringify(user))
}

export function removeUserInfo(): void {
  localStorage.removeItem(USER_KEY)
}
```

---

### Task 6: 创建 API 接口封装

**Files:**
- Create: `chat-web/src/api/auth.ts`
- Create: `chat-web/src/api/user.ts`
- Create: `chat-web/src/api/friends.ts`
- Create: `chat-web/src/api/groups.ts`
- Create: `chat-web/src/api/sessions.ts`
- Create: `chat-web/src/api/messages.ts`

**Step 1: 创建 auth.ts**

```typescript
import request from '@/utils/request'
import type { ApiResponse, LoginParams, SmsLoginParams, RegisterParams, User } from '@/types'

export function login(data: LoginParams) {
  return request.post<any, ApiResponse<{ accessToken: string; user: User }>>('/api/auth/login', data)
}

export function smsLogin(data: SmsLoginParams) {
  return request.post<any, ApiResponse<{ accessToken: string; user: User }>>('/api/auth/sms-login', data)
}

export function sendSmsCode(phone: string) {
  return request.post<any, ApiResponse>('/api/auth/sms-code', { phone })
}

export function register(data: RegisterParams) {
  return request.post<any, ApiResponse>('/api/auth/register', data)
}

export function refreshToken() {
  return request.post<any, ApiResponse<{ accessToken: string }>>('/api/auth/refresh')
}
```

**Step 2: 创建 user.ts**

```typescript
import request from '@/utils/request'
import type { ApiResponse, User } from '@/types'

export function getUserInfo() {
  return request.get<any, ApiResponse<User>>('/api/user/info')
}

export function updateUserInfo(data: Partial<User>) {
  return request.put<any, ApiResponse>('/api/user/info', data)
}

export function getPublicUserInfo(userId: number) {
  return request.get<any, ApiResponse<User>>('/api/user/public-info', { params: { userId } })
}

export function uploadAvatar(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  return request.post<any, ApiResponse<{ url: string }>>('/api/upload/avatar', formData, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
}
```

**Step 3: 创建 friends.ts**

```typescript
import request from '@/utils/request'
import type { ApiResponse, Friend, FriendApply, CursorResult } from '@/types'

export function getFriendList() {
  return request.get<any, ApiResponse<Friend[]>>('/api/friends')
}

export function getFriendInfo(friendId: number) {
  return request.get<any, ApiResponse<Friend>>('/api/friends/info', { params: { friendId } })
}

export function addFriend(toUserId: number, remark?: string) {
  return request.post<any, ApiResponse>('/api/friends/apply', { toUserId, remark })
}

export function deleteFriend(friendId: number) {
  return request.delete<any, ApiResponse>('/api/friends', { data: { friendId } })
}

export function blockFriend(friendId: number) {
  return request.post<any, ApiResponse>('/api/friends/block', { friendId })
}

export function unblockFriend(friendId: number) {
  return request.delete<any, ApiResponse>('/api/friends/block', { data: { friendId } })
}

export function updateFriendRemark(friendId: number, remark: string) {
  return request.put<any, ApiResponse>('/api/friends/remark', { friendId, remark })
}

export function getFriendApplyList() {
  return request.get<any, ApiResponse<FriendApply[]>>('/api/friends/applies')
}

export function passFriendApply(applyId: number) {
  return request.post<any, ApiResponse>('/api/friends/applies/approve', { applyId })
}

export function refuseFriendApply(applyId: number) {
  return request.post<any, ApiResponse>('/api/friends/applies/refuse', { applyId })
}
```

**Step 4: 创建 groups.ts**

```typescript
import request from '@/utils/request'
import type { ApiResponse, Group, GroupMember, GroupApply, CursorResult } from '@/types'

export function createGroup(groupName: string, userIds: number[]) {
  return request.post<any, ApiResponse<Group>>('/api/groups', { groupName, userIds })
}

export function getMyCreatedGroups() {
  return request.get<any, ApiResponse<Group[]>>('/api/groups/owned')
}

export function getMyJoinedGroups() {
  return request.get<any, ApiResponse<Group[]>>('/api/groups/joined')
}

export function getGroupDetail(groupId: number) {
  return request.get<any, ApiResponse<Group>>('/api/groups/detail', { params: { groupId } })
}

export function updateGroupInfo(groupId: number, groupName: string, avatar?: string) {
  return request.put<any, ApiResponse>('/api/groups/info', { groupId, groupName, avatar })
}

export function dismissGroup(groupId: number) {
  return request.delete<any, ApiResponse>('/api/groups', { data: { groupId } })
}

export function leaveGroup(groupId: number) {
  return request.post<any, ApiResponse>('/api/groups/leave', { groupId })
}

export function getGroupMembers(groupId: number) {
  return request.get<any, ApiResponse<GroupMember[]>>('/api/groups/members', { params: { groupId } })
}

export function removeGroupMembers(groupId: number, userIds: number[]) {
  return request.delete<any, ApiResponse>('/api/groups/members', { data: { groupId, userIds } })
}

export function muteGroupMember(groupId: number, userId: number, duration: number) {
  return request.post<any, ApiResponse>('/api/groups/members/mute', { groupId, userId, duration })
}

export function applyJoinGroup(groupId: number, remark?: string) {
  return request.post<any, ApiResponse>('/api/groups/apply', { groupId, remark })
}

export function getGroupApplyList(groupId: number) {
  return request.get<any, ApiResponse<GroupApply[]>>('/api/groups/applies', { params: { groupId } })
}

export function passGroupApply(applyId: number) {
  return request.post<any, ApiResponse>('/api/groups/applies/approve', { applyId })
}

export function refuseGroupApply(applyId: number) {
  return request.post<any, ApiResponse>('/api/groups/applies/refuse', { applyId })
}
```

**Step 5: 创建 sessions.ts**

```typescript
import request from '@/utils/request'
import type { ApiResponse, Session } from '@/types'

export function checkSessionAllowed(targetId: number, sessionType: number) {
  return request.get<any, ApiResponse<{ allowed: boolean }>>('/api/sessions/check', { params: { targetId, sessionType } })
}

export function openSession(targetId: number, sessionType: number) {
  return request.post<any, ApiResponse<Session>>('/api/sessions', { targetId, sessionType })
}

export function getUserSessionList() {
  return request.get<any, ApiResponse<Session[]>>('/api/sessions/direct')
}

export function getGroupSessionList() {
  return request.get<any, ApiResponse<Session[]>>('/api/sessions/group')
}

export function deleteSession(sessionId: number) {
  return request.delete<any, ApiResponse>('/api/sessions', { data: { sessionId } })
}

export function pinSession(sessionId: number, isPinned: boolean) {
  return request.put<any, ApiResponse>('/api/sessions/pin', { sessionId, isPinned })
}
```

**Step 6: 创建 messages.ts**

```typescript
import request from '@/utils/request'
import type { ApiResponse, Message, CursorResult } from '@/types'

export function getMessageList(sessionId: number, cursor?: string, limit: number = 20) {
  return request.get<any, ApiResponse<CursorResult<Message>>>('/api/messages', { 
    params: { sessionId, cursor, limit } 
  })
}

export function recallMessage(messageId: number) {
  return request.post<any, ApiResponse>('/api/messages/recall', { messageId })
}

export function uploadFile(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  return request.post<any, ApiResponse<{ url: string }>>('/api/upload/file', formData, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
}
```

---

### Task 7: 创建状态管理 (Pinia Stores)

**Files:**
- Create: `chat-web/src/stores/user.ts`
- Create: `chat-web/src/stores/chat.ts`
- Create: `chat-web/src/stores/contact.ts`

**Step 1: 创建 user store**

```typescript
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { User } from '@/types'
import { getToken, setToken, removeToken, getUserInfo, setUserInfo, removeUserInfo } from '@/utils/storage'
import * as authApi from '@/api/auth'
import * as userApi from '@/api/user'

export const useUserStore = defineStore('user', () => {
  const token = ref(getToken() || '')
  const userInfo = ref<User | null>(getUserInfo())

  const isAuthenticated = computed(() => !!token.value)

  async function login(phone: string, password: string) {
    const data = await authApi.login({ phone, password })
    token.value = data.accessToken
    userInfo.value = data.user
    setToken(data.accessToken)
    setUserInfo(data.user)
  }

  async function smsLogin(phone: string, code: string) {
    const data = await authApi.smsLogin({ phone, code })
    token.value = data.accessToken
    userInfo.value = data.user
    setToken(data.accessToken)
    setUserInfo(data.user)
  }

  async function register(phone: string, password: string, nickname: string, code: string) {
    await authApi.register({ phone, password, nickname, code })
  }

  async function fetchUserInfo() {
    const data = await userApi.getUserInfo()
    userInfo.value = data
    setUserInfo(data)
  }

  async function updateUserInfo(info: Partial<User>) {
    await userApi.updateUserInfo(info)
    await fetchUserInfo()
  }

  function logout() {
    token.value = ''
    userInfo.value = null
    removeToken()
    removeUserInfo()
  }

  return {
    token,
    userInfo,
    isAuthenticated,
    login,
    smsLogin,
    register,
    fetchUserInfo,
    updateUserInfo,
    logout
  }
})
```

**Step 2: 创建 chat store**

```typescript
import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Session, Message } from '@/types'
import * as sessionApi from '@/api/sessions'
import * as messageApi from '@/api/messages'

export const useChatStore = defineStore('chat', () => {
  const sessions = ref<Session[]>([])
  const currentSession = ref<Session | null>(null)
  const messages = ref<Message[]>([])
  const loading = ref(false)

  async function loadUserSessions() {
    const [direct, group] = await Promise.all([
      sessionApi.getUserSessionList(),
      sessionApi.getGroupSessionList()
    ])
    sessions.value = [...direct, ...group].sort((a, b) => 
      new Date(b.createTime).getTime() - new Date(a.createTime).getTime()
    )
  }

  async function openSession(targetId: number, sessionType: number) {
    const session = await sessionApi.openSession(targetId, sessionType)
    const index = sessions.value.findIndex(s => s.sessionId === session.sessionId)
    if (index === -1) {
      sessions.value.unshift(session)
    } else {
      sessions.value[index] = session
    }
    currentSession.value = session
    await loadMessages(session.sessionId)
  }

  async function loadMessages(sessionId: number, cursor?: string) {
    loading.value = true
    try {
      const data = await messageApi.getMessageList(sessionId, cursor)
      if (cursor) {
        messages.value = [...data.list.reverse(), ...messages.value]
      } else {
        messages.value = data.list.reverse()
      }
      return data
    } finally {
      loading.value = false
    }
  }

  function addMessage(message: Message) {
    messages.value.push(message)
    // 更新会话最后消息
    const session = sessions.value.find(s => s.sessionId === message.sessionId)
    if (session) {
      session.lastMessage = message
    }
  }

  async function recallMessage(messageId: number) {
    await messageApi.recallMessage(messageId)
    const msg = messages.value.find(m => m.messageId === messageId)
    if (msg) {
      msg.isRecall = 1
    }
  }

  async function deleteSession(sessionId: number) {
    await sessionApi.deleteSession(sessionId)
    sessions.value = sessions.value.filter(s => s.sessionId !== sessionId)
    if (currentSession.value?.sessionId === sessionId) {
      currentSession.value = null
      messages.value = []
    }
  }

  async function pinSession(sessionId: number, isPinned: boolean) {
    await sessionApi.pinSession(sessionId, isPinned)
    const session = sessions.value.find(s => s.sessionId === sessionId)
    if (session) {
      session.isPinned = isPinned ? 1 : 0
    }
  }

  return {
    sessions,
    currentSession,
    messages,
    loading,
    loadUserSessions,
    openSession,
    loadMessages,
    addMessage,
    recallMessage,
    deleteSession,
    pinSession
  }
})
```

**Step 3: 创建 contact store**

```typescript
import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Friend, Group, FriendApply, GroupApply } from '@/types'
import * as friendApi from '@/api/friends'
import * as groupApi from '@/api/groups'

export const useContactStore = defineStore('contact', () => {
  const friends = ref<Friend[]>([])
  const myGroups = ref<Group[]>(value = [])
  const joinedGroups = ref<Group[]>([])
  const friendApplies = ref<FriendApply[]>([])
  const groupApplies = ref<GroupApply[]>([])
  const loading = ref(false)

  async function loadFriends() {
    loading.value = true
    try {
      friends.value = await friendApi.getFriendList()
    } finally {
      loading.value = false
    }
  }

  async function loadMyGroups() {
    const [owned, joined] = await Promise.all([
      groupApi.getMyCreatedGroups(),
      groupApi.getMyJoinedGroups()
    ])
    myGroups.value = owned
    joinedGroups.value = joined
  }

  async function loadFriendApplies() {
    friendApplies.value = await friendApi.getFriendApplyList()
  }

  async function loadGroupApplies(groupId: number) {
    groupApplies.value = await groupApi.getGroupApplyList(groupId)
  }

  async function addFriend(toUserId: number, remark?: string) {
    await friendApi.addFriend(toUserId, remark)
  }

  async function deleteFriend(friendId: number) {
    await friendApi.deleteFriend(friendId)
    friends.value = friends.value.filter(f => f.friendId !== friendId)
  }

  async function createGroup(groupName: string, userIds: number[]) {
    const group = await groupApi.createGroup(groupName, userIds)
    myGroups.value.push(group)
    return group
  }

  async function joinGroup(groupId: number, remark?: string) {
    await groupApi.applyJoinGroup(groupId, remark)
  }

  async function leaveGroup(groupId: number) {
    await groupApi.leaveGroup(groupId)
    joinedGroups.value = joinedGroups.value.filter(g => g.groupId !== groupId)
    myGroups.value = myGroups.value.filter(g => g.groupId !== groupId)
  }

  return {
    friends,
    myGroups,
    joinedGroups,
    friendApplies,
    groupApplies,
    loading,
    loadFriends,
    loadMyGroups,
    loadFriendApplies,
    loadGroupApplies,
    addFriend,
    deleteFriend,
    createGroup,
    joinGroup,
    leaveGroup
  }
})
```

---

### Task 8: 创建 WebSocket Composable

**Files:**
- Create: `chat-web/src/composables/useWebSocket.ts`

**Step 1: 创建 WebSocket Hook**

```typescript
import { ref, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/stores/user'
import { useChatStore } from '@/stores/chat'
import type { Message } from '@/types'

export function useWebSocket() {
  const ws = ref<WebSocket | null>(null)
  const connected = ref(false)
  const reconnecting = ref(false)
  
  const userStore = useUserStore()
  const chatStore = useChatStore()

  function connect() {
    if (ws.value?.readyState === WebSocket.OPEN) return
    
    const wsUrl = `${import.meta.env.VITE_APP_WS_URL}?token=${userStore.token}`
    ws.value = new WebSocket(wsUrl)

    ws.value.onopen = () => {
      connected.value = true
      reconnecting.value = false
      console.log('WebSocket connected')
    }

    ws.value.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        handleMessage(data)
      } catch (e) {
        console.error('Parse message error:', e)
      }
    }

    ws.value.onclose = () => {
      connected.value = false
      console.log('WebSocket closed')
      reconnect()
    }

    ws.value.onerror = (error) => {
      console.error('WebSocket error:', error)
    }
  }

  function handleMessage(data: any) {
    switch (data.type) {
      case 'message':
        chatStore.addMessage(data.message)
        break
      case 'recall':
        const msg = chatStore.messages.find(m => m.messageId === data.messageId)
        if (msg) msg.isRecall = 1
        break
      case 'friend_apply':
        ElMessage.info('收到新的好友申请')
        break
      case 'group_apply':
        ElMessage.info('收到新的入群申请')
        break
      case 'system':
        ElMessage.info(data.content)
        break
    }
  }

  function send(data: any) {
    if (ws.value?.readyState === WebSocket.OPEN) {
      ws.value.send(JSON.stringify(data))
    }
  }

  function reconnect() {
    if (reconnecting.value) return
    reconnecting.value = true
    setTimeout(() => {
      if (userStore.isAuthenticated) {
        connect()
      }
      reconnecting.value = false
    }, 3000)
  }

  function disconnect() {
    ws.value?.close()
    ws.value = null
    connected.value = false
  }

  // 心跳
  let heartbeatInterval: number
  function startHeartbeat() {
    heartbeatInterval = window.setInterval(() => {
      send({ type: 'ping' })
    }, 30000)
  }

  function stopHeartbeat() {
    clearInterval(heartbeatInterval)
  }

  onUnmounted(() => {
    disconnect()
    stopHeartbeat()
  })

  return {
    connected,
    connect,
    disconnect,
    send,
    startHeartbeat,
    stopHeartbeat
  }
}
```

---

### Task 9: 创建登录页面

**Files:**
- Create: `chat-web/src/views/Login.vue`

**Step 1: 创建登录页面**

```vue
<template>
  <div class="login-page">
    <div class="login-card">
      <h1 class="title">KamaChat</h1>
      <el-form :model="form" :rules="rules" ref="formRef">
        <el-form-item prop="phone">
          <el-input 
            v-model="form.phone" 
            placeholder="手机号"
            :prefix-icon="Phone"
            size="large"
          />
        </el-form-item>
        <el-form-item prop="password">
          <el-input 
            v-model="form.password" 
            type="password"
            placeholder="密码"
            :prefix-icon="Lock"
            size="large"
            show-password
            @keyup.enter="handleLogin"
          />
        </el-form-item>
        <el-form-item>
          <el-button 
            type="primary" 
            size="large" 
            :loading="loading"
            @click="handleLogin"
            style="width: 100%"
          >
            登录
          </el-button>
        </el-form-item>
      </el-form>
      <div class="links">
        <el-link type="primary" @click="$router.push('/sms-login')">短信验证码登录</el-link>
        <el-link type="primary" @click="$router.push('/register')">立即注册</el-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, FormInstance } from 'element-plus'
import { Phone, Lock } from '@element-plus/icons-vue'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const userStore = useUserStore()

const formRef = ref<FormInstance>()
const loading = ref(false)

const form = reactive({
  phone: '',
  password: ''
})

const rules = {
  phone: [{ required: true, message: '请输入手机号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

async function handleLogin() {
  await formRef.value?.validate()
  loading.value = true
  try {
    await userStore.login(form.phone, form.password)
    ElMessage.success('登录成功')
    router.push('/home')
  } catch (e: any) {
    ElMessage.error(e.message || '登录失败')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped lang="scss">
.login-page {
  width: 100%;
  height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.login-card {
  width: 400px;
  padding: 40px;
  background: white;
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
}

.title {
  text-align: center;
  font-size: 28px;
  font-weight: 600;
  color: #333;
  margin-bottom: 30px;
}

.links {
  display: flex;
  justify-content: space-between;
  margin-top: 10px;
}
</style>
```

---

### Task 10: 创建注册页面

**Files:**
- Create: `chat-web/src/views/Register.vue`

**Step 1: 创建注册页面**

```vue
<template>
  <div class="register-page">
    <div class="register-card">
      <h1 class="title">注册账号</h1>
      <el-form :model="form" :rules="rules" ref="formRef">
        <el-form-item prop="nickname">
          <el-input v-model="form.nickname" placeholder="昵称" :prefix-icon="User" size="large" />
        </el-form-item>
        <el-form-item prop="phone">
          <el-input v-model="form.phone" placeholder="手机号" :prefix-icon="Phone" size="large" />
        </el-form-item>
        <el-form-item prop="password">
          <el-input v-model="form.password" type="password" placeholder="密码" :prefix-icon="Lock" size="large" show-password />
        </el-form-item>
        <el-form-item prop="code">
          <div class="code-input">
            <el-input v-model="form.code" placeholder="验证码" size="large" @keyup.enter="handleRegister" />
            <el-button size="large" :disabled="countdown > 0" @click="sendCode">
              {{ countdown > 0 ? `${countdown}s` : '获取验证码' }}
            </el-button>
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" size="large" :loading="loading" @click="handleRegister" style="width: 100%">
            注册
          </el-button>
        </el-form-item>
      </el-form>
      <div class="links">
        <el-link type="primary" @click="$router.push('/login')">已有账号？立即登录</el-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, FormInstance } from 'element-plus'
import { User, Phone, Lock } from '@element-plus/icons-vue'
import { useUserStore } from '@/stores/user'
import { sendSmsCode } from '@/api/auth'

const router = useRouter()
const userStore = useUserStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const countdown = ref(0)

const form = reactive({
  nickname: '',
  phone: '',
  password: '',
  code: ''
})

const rules = {
  nickname: [{ required: true, message: '请输入昵称', trigger: 'blur' }],
  phone: [{ required: true, message: '请输入手机号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }, { min: 6, message: '密码至少6位', trigger: 'blur' }],
  code: [{ required: true, message: '请输入验证码', trigger: 'blur' }]
}

async function sendCode() {
  if (!form.phone) {
    ElMessage.warning('请先输入手机号')
    return
  }
  await sendSmsCode(form.phone)
  ElMessage.success('验证码已发送')
  countdown.value = 60
  const timer = setInterval(() => {
    countdown.value--
    if (countdown.value <= 0) clearInterval(timer)
  }, 1000)
}

async function handleRegister() {
  await formRef.value?.validate()
  loading.value = true
  try {
    await userStore.register(form.phone, form.password, form.nickname, form.code)
    ElMessage.success('注册成功，请登录')
    router.push('/login')
  } catch (e: any) {
    ElMessage.error(e.message || '注册失败')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped lang="scss">
.register-page {
  width: 100%;
  height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.register-card {
  width: 400px;
  padding: 40px;
  background: white;
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
}

.title {
  text-align: center;
  font-size: 24px;
  font-weight: 600;
  color: #333;
  margin-bottom: 30px;
}

.code-input {
  display: flex;
  gap: 10px;
  :deep(.el-input) {
    flex: 1;
  }
}

.links {
  text-align: center;
  margin-top: 10px;
}
</style>
```

---

### Task 11: 创建短信登录页面

**Files:**
- Create: `chat-web/src/views/SmsLogin.vue`

**Step 1: 创建短信登录页面**

```vue
<template>
  <div class="sms-login-page">
    <div class="login-card">
      <h1 class="title">短信验证码登录</h1>
      <el-form :model="form" :rules="rules" ref="formRef">
        <el-form-item prop="phone">
          <el-input v-model="form.phone" placeholder="手机号" :prefix-icon="Phone" size="large" />
        </el-form-item>
        <el-form-item prop="code">
          <div class="code-input">
            <el-input v-model="form.code" placeholder="验证码" size="large" @keyup.enter="handleLogin" />
            <el-button size="large" :disabled="countdown > 0" @click="sendCode">
              {{ countdown > 0 ? `${countdown}s` : '获取验证码' }}
            </el-button>
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" size="large" :loading="loading" @click="handleLogin" style="width: 100%">
            登录
          </el-button>
        </el-form-item>
      </el-form>
      <div class="links">
        <el-link type="primary" @click="$router.push('/login')">密码登录</el-link>
        <el-link type="primary" @click="$router.push('/register')">立即注册</el-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, FormInstance } from 'element-plus'
import { Phone } from '@element-plus/icons-vue'
import { useUserStore } from '@/stores/user'
import { sendSmsCode } from '@/api/auth'

const router = useRouter()
const userStore = useUserStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const countdown = ref(0)

const form = reactive({
  phone: '',
  code: ''
})

const rules = {
  phone: [{ required: true, message: '请输入手机号', trigger: 'blur' }],
  code: [{ required: true, message: '请输入验证码', trigger: 'blur' }]
}

async function sendCode() {
  if (!form.phone) {
    ElMessage.warning('请先输入手机号')
    return
  }
  await sendSmsCode(form.phone)
  ElMessage.success('验证码已发送')
  countdown.value = 60
  const timer = setInterval(() => {
    countdown.value--
    if (countdown.value <= 0) clearInterval(timer)
  }, 1000)
}

async function handleLogin() {
  await formRef.value?.validate()
  loading.value = true
  try {
    await userStore.smsLogin(form.phone, form.code)
    ElMessage.success('登录成功')
    router.push('/home')
  } catch (e: any) {
    ElMessage.error(e.message || '登录失败')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped lang="scss">
.sms-login-page {
  width: 100%;
  height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.login-card {
  width: 400px;
  padding: 40px;
  background: white;
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
}

.title {
  text-align: center;
  font-size: 24px;
  font-weight: 600;
  color: #333;
  margin-bottom: 30px;
}

.code-input {
  display: flex;
  gap: 10px;
  :deep(.el-input) {
    flex: 1;
  }
}

.links {
  display: flex;
  justify-content: space-between;
  margin-top: 10px;
}
</style>
```

---

### Task 12: 创建主页面框架

**Files:**
- Create: `chat-web/src/views/Home.vue`

**Step 1: 创建 Home.vue**

```vue
<template>
  <div class="home-page">
    <div class="sidebar">
      <div class="user-info" @click="$router.push('/home/profile')">
        <el-avatar :size="40" :src="userStore.userInfo?.avatar || defaultAvatar" />
        <span class="nickname">{{ userStore.userInfo?.nickname }}</span>
      </div>
      <div class="nav-tabs">
        <div 
          v-for="tab in tabs" 
          :key="tab.path"
          class="nav-item"
          :class="{ active: currentTab === tab.path }"
          @click="currentTab = tab.path"
        >
          <el-icon :size="24"><component :is="tab.icon" /></el-icon>
          <span>{{ tab.label }}</span>
          <el-badge v-if="tab.badge && tab.badge > 0" :value="tab.badge" class="badge" />
        </div>
      </div>
    </div>
    <div class="main-content">
      <RouterView />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ChatDotRound, User, FolderOpened, UserFilled } from '@element-plus/icons-vue'
import { useUserStore } from '@/stores/user'
import { useChatStore } from '@/stores/chat'
import { useContactStore } from '@/stores/contact'
import { useWebSocket } from '@/composables/useWebSocket'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const chatStore = useChatStore()
const contactStore = useContactStore()

const { connect, startHeartbeat } = useWebSocket()

const defaultAvatar = 'https://cube.elemecdn.com/3/7c/3ea6beec64369c2642b92c6726f1epng.png'

const currentTab = computed(() => {
  const path = route.path
  if (path.includes('conversations')) return '/home/conversations'
  if (path.includes('friends')) return '/home/friends'
  if (path.includes('groups')) return '/home/groups'
  if (path.includes('profile')) return '/home/profile'
  return '/home/conversations'
})

const tabs = computed(() => [
  { path: '/home/conversations', label: '会话', icon: ChatDotRound, badge: 0 },
  { path: '/home/friends', label: '好友', icon: User, badge: contactStore.friendApplies.length },
  { path: '/home/groups', label: '群组', icon: FolderOpened, badge: 0 },
  { path: '/home/profile', label: '我的', icon: UserFilled, badge: 0 }
])

onMounted(async () => {
  // 加载数据
  await Promise.all([
    userStore.fetchUserInfo(),
    chatStore.loadUserSessions(),
    contactStore.loadFriends(),
    contactStore.loadMyGroups()
  ])
  
  // 连接 WebSocket
  connect()
  startHeartbeat()
})
</script>

<style scoped lang="scss">
.home-page {
  width: 100%;
  height: 100vh;
  display: flex;
}

.sidebar {
  width: 80px;
  background: #2e2e2e;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 20px 0;
}

.user-info {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 10px 0;
  cursor: pointer;
  
  .nickname {
    color: #fff;
    font-size: 12px;
    margin-top: 8px;
    text-overflow: ellipsis;
    overflow: hidden;
    white-space: nowrap;
    width: 70px;
    text-align: center;
  }
}

.nav-tabs {
  margin-top: 30px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.nav-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  color: #8c8c8c;
  cursor: pointer;
  position: relative;
  
  &.active {
    color: #409eff;
  }
  
  span {
    font-size: 12px;
    margin-top: 4px;
  }
  
  .badge {
    position: absolute;
    top: -5px;
    right: 10px;
  }
}

.main-content {
  flex: 1;
  background: #f5f5f5;
}
</style>
```

---

### Task 13: 创建会话列表页面

**Files:**
- Create: `chat-web/src/views/conversations/index.vue`

**Step 1: 创建会话列表页面**

```vue
<template>
  <div class="conversations-page">
    <div class="session-list">
      <div class="header">
        <el-input v-model="searchKey" placeholder="搜索会话" :prefix-icon="Search" clearable />
      </div>
      <div class="list">
        <div 
          v-for="session in filteredSessions" 
          :key="session.sessionId"
          class="session-item"
          :class="{ active: chatStore.currentSession?.sessionId === session.sessionId }"
          @click="handleSelectSession(session)"
        >
          <el-avatar :size="44" :src="session.avatar || defaultAvatar" />
          <div class="content">
            <div class="top">
              <span class="name">{{ session.name }}</span>
              <span class="time">{{ formatTime(session.lastMessage?.createTime) }}</span>
            </div>
            <div class="bottom">
              <span class="preview">{{ session.lastMessage?.isRecall ? '已撤回消息' : session.lastMessage?.content }}</span>
              <el-badge v-if="session.unreadCount > 0" :value="session.unreadCount" />
            </div>
          </div>
        </div>
        <el-empty v-if="filteredSessions.length === 0" description="暂无会话" />
      </div>
    </div>
    <div class="chat-area">
      <ChatWindow v-if="chatStore.currentSession" />
      <el-empty v-else description="选择一个会话开始聊天" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { useChatStore } from '@/stores/chat'
import ChatWindow from '@/components/ChatWindow.vue'
import type { Session } from '@/types'

const chatStore = useChatStore()
const searchKey = ref('')
const defaultAvatar = 'https://cube.elemecdn.com/3/7c/3ea6beec64369c2642b92c6726f1epng.png'

const filteredSessions = computed(() => {
  if (!searchKey.value) return chatStore.sessions
  return chatStore.sessions.filter(s => s.name?.includes(searchKey.value))
})

async function handleSelectSession(session: Session) {
  await chatStore.openSession(session.targetId, session.sessionType)
}

function formatTime(time?: string) {
  if (!time) return ''
  const date = new Date(time)
  const now = new Date()
  const diff = now.getTime() - date.getTime()
  
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}小时前`
  return time.split('T')[0]
}
</script>

<style scoped lang="scss">
.conversations-page {
  width: 100%;
  height: 100%;
  display: flex;
}

.session-list {
  width: 320px;
  background: #fff;
  border-right: 1px solid #e8e8e8;
  display: flex;
  flex-direction: column;
}

.header {
  padding: 15px;
  border-bottom: 1px solid #e8e8e8;
}

.list {
  flex: 1;
  overflow-y: auto;
}

.session-item {
  display: flex;
  padding: 12px 15px;
  cursor: pointer;
  transition: background 0.2s;
  
  &:hover {
    background: #f5f5f5;
  }
  
  &.active {
    background: #e8f3ff;
  }
  
  .content {
    flex: 1;
    margin-left: 12px;
    overflow: hidden;
  }
  
  .top {
    display: flex;
    justify-content: space-between;
    align-items: center;
    
    .name {
      font-size: 14px;
      font-weight: 500;
      color: #333;
    }
    
    .time {
      font-size: 12px;
      color: #999;
    }
  }
  
  .bottom {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-top: 4px;
    
    .preview {
      font-size: 12px;
      color: #999;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      flex: 1;
    }
  }
}

.chat-area {
  flex: 1;
  display: flex;
  flex-direction: column;
}
</style>
```

---

### Task 14: 创建聊天窗口组件

**Files:**
- Create: `chat-web/src/components/ChatWindow.vue`

**Step 1: 创建聊天窗口组件**

```vue
<template>
  <div class="chat-window">
    <div class="chat-header">
      <span class="title">{{ chatStore.currentSession?.name }}</span>
    </div>
    <div class="message-list" ref="listRef" @scroll="handleScroll">
      <div v-if="loading" class="loading-tip">加载中...</div>
      <div v-for="msg in chatStore.messages" :key="msg.messageId" class="message-item" :class="{ self: msg.senderId === userStore.userInfo?.userId }">
        <el-avatar :size="36" :src="getSenderAvatar(msg)" />
        <div class="message-content">
          <div class="bubble">
            <template v-if="msg.messageType === 1">
              <span v-if="msg.isRecall" class="recall-text">你撤回了一条消息</span>
              <span v-else>{{ msg.content }}</span>
            </template>
            <template v-else-if="msg.messageType === 2">
              <img :src="msg.url" class="message-image" @click="previewImage(msg.url)" />
            </template>
            <template v-else-if="msg.messageType === 3">
              <a :href="msg.url" download class="file-link">
                <el-icon><Document /></el-icon>
                {{ msg.extra }}
              </a>
            </template>
          </div>
        </div>
      </div>
    </div>
    <div class="chat-input">
      <div class="toolbar">
        <el-icon @click="showEmoji = !showEmoji"><Smile /></el-icon>
        <el-icon @click="handleUploadFile"><Picture /></el-icon>
        <el-icon @click="handleUploadFile(true)"><Document /></el-icon>
      </div>
      <div class="input-area">
        <el-input 
          v-model="inputText" 
          type="textarea" 
          :rows="3" 
          placeholder="输入消息..."
          @keydown.enter.exact.prevent="handleSend"
          resize="none"
        />
        <el-button type="primary" @click="handleSend" :disabled="!inputText.trim()">发送</el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick, onMounted } from 'vue'
import { Smile, Picture, Document } from '@element-plus/icons-vue'
import { useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import { sendMessage } from '@/composables/useWebSocket'
import * as messageApi from '@/api/messages'

const chatStore = useChatStore()
const userStore = useUserStore()

const inputText = ref('')
const listRef = ref<HTMLElement>()
const loading = ref(false)
const showEmoji = ref(false)

const defaultAvatar = 'https://cube.elemecdn.com/3/7c/3ea6beec64369c2642b92c6726f1epng.png'

function getSenderAvatar(msg: any) {
  // TODO: 根据 senderId 获取用户头像
  return defaultAvatar
}

async function handleSend() {
  if (!inputText.value.trim() || !chatStore.currentSession) return
  
  const content = inputText.value.trim()
  inputText.value = ''
  
  // 发送 WebSocket 消息
  sendMessage({
    type: 'message',
    sessionId: chatStore.currentSession.sessionId,
    sessionType: chatStore.currentSession.sessionType,
    targetId: chatStore.currentSession.targetId,
    messageType: 1,
    content
  })
}

async function handleUploadFile(isFile = false) {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = isFile ? '*' : 'image/*'
  input.onchange = async () => {
    if (!input.files?.length) return
    const file = input.files[0]
    const { url } = await messageApi.uploadFile(file)
    
    sendMessage({
      type: 'message',
      sessionId: chatStore.currentSession!.sessionId,
      sessionType: chatStore.currentSession!.sessionType,
      targetId: chatStore.currentSession!.targetId,
      messageType: isFile ? 3 : 2,
      url,
      extra: file.name
    })
  }
  input.click()
}

function handleScroll() {
  if (listRef.value?.scrollTop === 0 && !loading.value && chatStore.currentSession) {
    loading.value = true
    const firstMsg = chatStore.messages[0]
    messageApi.getMessageList(chatStore.currentSession.sessionId, firstMsg?.messageId.toString()).then(data => {
      chatStore.messages = [...data.list.reverse(), ...chatStore.messages]
    }).finally(() => {
      loading.value = false
    })
  }
}

function previewImage(url: string) {
  // TODO: 图片预览
}

onMounted(() => {
  nextTick(() => {
    if (listRef.value) {
      listRef.value.scrollTop = listRef.value.scrollHeight
    }
  })
})
</script>

<style scoped lang="scss">
.chat-window {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #fff;
}

.chat-header {
  height: 50px;
  padding: 0 20px;
  border-bottom: 1px solid #e8e8e8;
  display: flex;
  align-items: center;
  
  .title {
    font-size: 16px;
    font-weight: 500;
  }
}

.message-list {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
  
  .loading-tip {
    text-align: center;
    color: #999;
    padding: 10px;
  }
}

.message-item {
  display: flex;
  margin-bottom: 20px;
  
  &.self {
    flex-direction: row-reverse;
    
    .message-content {
      margin-right: 10px;
      margin-left: 0;
      
      .bubble {
        background: #409eff;
        color: #fff;
      }
    }
  }
  
  .message-content {
    margin-left: 10px;
    max-width: 60%;
  }
  
  .bubble {
    padding: 10px 14px;
    background: #f5f5f5;
    border-radius: 8px;
    word-break: break-word;
    
    .message-image {
      max-width: 200px;
      max-height: 200px;
      border-radius: 4px;
      cursor: pointer;
    }
    
    .file-link {
      display: flex;
      align-items: center;
      gap: 8px;
      color: #409eff;
      text-decoration: none;
    }
    
    .recall-text {
      color: #999;
      font-size: 12px;
      font-style: italic;
    }
  }
}

.chat-input {
  border-top: 1px solid #e8e8e8;
  padding: 10px 20px;
  
  .toolbar {
    display: flex;
    gap: 15px;
    margin-bottom: 10px;
    color: #666;
    
    .el-icon {
      cursor: pointer;
      font-size: 20px;
      
      &:hover {
        color: #409eff;
      }
    }
  }
  
  .input-area {
    display: flex;
    gap: 10px;
    align-items: flex-end;
    
    :deep(.el-textarea) {
      flex: 1;
    }
  }
}
</style>
```

---

### Task 15: 创建好友列表页面

**Files:**
- Create: `chat-web/src/views/friends/index.vue`

**Step 1: 创建好友列表页面**

```vue
<template>
  <div class="friends-page">
    <div class="header">
      <el-input v-model="searchKey" placeholder="搜索好友" :prefix-icon="Search" clearable />
      <el-button type="primary" @click="showAddDialog = true">添加好友</el-button>
    </div>
    
    <el-tabs v-model="activeTab">
      <el-tab-pane label="好友列表" name="list">
        <div class="friend-list">
          <div v-for="friend in contactStore.friends" :key="friend.friendId" class="friend-item">
            <el-avatar :size="48" :src="friend.user.avatar || defaultAvatar" />
            <div class="info">
              <div class="name">{{ friend.remark || friend.user.nickname }}</div>
              <div class="phone">{{ friend.user.phone }}</div>
            </div>
            <div class="actions">
              <el-button size="small" @click="startChat(friend)">发消息</el-button>
              <el-button size="small" @click="showRemarkDialog(friend)">备注</el-button>
              <el-button size="small" type="danger" @click="handleDelete(friend.friendId)">删除</el-button>
            </div>
          </div>
          <el-empty v-if="contactStore.friends.length === 0" description="暂无好友" />
        </div>
      </el-tab-pane>
      
      <el-tab-pane label="好友申请" name="apply">
        <div class="apply-list">
          <div v-for="apply in contactStore.friendApplies" :key="apply.applyId" class="apply-item">
            <el-avatar :size="48" :src="apply.fromUser.avatar || defaultAvatar" />
            <div class="info">
              <div class="name">{{ apply.fromUser.nickname }}</div>
              <div class="remark">{{ apply.remark || '申请添加你为好友' }}</div>
            </div>
            <div class="actions" v-if="apply.status === 0">
              <el-button size="small" type="primary" @click="handlePassApply(apply.applyId)">同意</el-button>
              <el-button size="small" @click="handleRefuseApply(apply.applyId)">拒绝</el-button>
            </div>
          </div>
          <el-empty v-if="contactStore.friendApplies.length === 0" description="暂无申请" />
        </div>
      </el-tab-pane>
    </el-tabs>
    
    <!-- 添加好友对话框 -->
    <el-dialog v-model="showAddDialog" title="添加好友" width="400px">
      <el-form :model="addForm" :rules="addRules" ref="addFormRef">
        <el-form-item prop="phone">
          <el-input v-model="addForm.phone" placeholder="请输入好友手机号" />
        </el-form-item>
        <el-form-item prop="remark">
          <el-input v-model="addForm.remark" placeholder="验证信息" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddDialog = false">取消</el-button>
        <el-button type="primary" @click="handleAddFriend">发送</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import { useContactStore } from '@/stores/contact'
import { useChatStore } from '@/stores/chat'
import * as friendApi from '@/api/friends'
import type { Friend, FriendApply } from '@/types'

const router = useRouter()
const contactStore = useContactStore()
const chatStore = useChatStore()

const searchKey = ref('')
const activeTab = ref('list')
const showAddDialog = ref(false)
const defaultAvatar = 'https://cube.elemecdn.com/3/7c/3ea6beec64369c2642b92c6726f1epng.png'

const addForm = reactive({ phone: '', remark: '' })
const addRules = { phone: [{ required: true, message: '请输入手机号', trigger: 'blur' }] }
const addFormRef = ref()

onMounted(() => {
  contactStore.loadFriendApplies()
})

async function startChat(friend: Friend) {
  await chatStore.openSession(friend.user.userId, 1)
  router.push('/home/conversations')
}

async function handleAddFriend() {
  await addFormRef.value?.validate()
  await contactStore.addFriend(addForm.phone, addForm.remark)
  ElMessage.success('已发送好友申请')
  showAddDialog.value = false
}

async function handleDelete(friendId: number) {
  await ElMessageBox.confirm('确定删除该好友吗？', '提示')
  await contactStore.deleteFriend(friendId)
  ElMessage.success('删除成功')
}

async function handlePassApply(applyId: number) {
  await friendApi.passFriendApply(applyId)
  ElMessage.success('已同意')
  contactStore.loadFriendApplies()
  contactStore.loadFriends()
}

async function handleRefuseApply(applyId: number) {
  await friendApi.refuseFriendApply(applyId)
  ElMessage.success('已拒绝')
  contactStore.loadFriendApplies()
}
</script>

<style scoped lang="scss">
.friends-page {
  height: 100%;
  background: #fff;
  display: flex;
  flex-direction: column;
}

.header {
  padding: 15px 20px;
  border-bottom: 1px solid #e8e8e8;
  display: flex;
  gap: 10px;
  
  :deep(.el-input) {
    flex: 1;
  }
}

:deep(.el-tabs__content) {
  flex: 1;
  overflow-y: auto;
}

.friend-list, .apply-list {
  padding: 10px 20px;
}

.friend-item, .apply-item {
  display: flex;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid #f0f0f0;
  
  .info {
    flex: 1;
    margin-left: 12px;
    
    .name {
      font-size: 14px;
      font-weight: 500;
    }
    
    .phone, .remark {
      font-size: 12px;
      color: #999;
      margin-top: 4px;
    }
  }
  
  .actions {
    display: flex;
    gap: 8px;
  }
}
</style>
```

---

### Task 16: 创建群组列表页面

**Files:**
- Create: `chat-web/src/views/groups/index.vue`

**Step 1: 创建群组列表页面**

```vue
<template>
  <div class="groups-page">
    <div class="header">
      <el-input v-model="searchKey" placeholder="搜索群组" :prefix-icon="Search" clearable />
      <el-button type="primary" @click="showCreateDialog = true">创建群组</el-button>
    </div>
    
    <el-tabs v-model="activeTab">
      <el-tab-pane label="我创建的" name="owned">
        <div class="group-list">
          <div v-for="group in contactStore.myGroups" :key="group.groupId" class="group-item">
            <el-avatar :size="48" :src="group.avatar || defaultGroupAvatar" />
            <div class="info">
              <div class="name">{{ group.groupName }}</div>
              <div class="member-count">{{ group.memberCount }} 人</div>
            </div>
            <div class="actions">
              <el-button size="small" @click="startChat(group)">发消息</el-button>
              <el-button size="small" @click="showGroupDetail(group)">管理</el-button>
            </div>
          </div>
          <el-empty v-if="contactStore.myGroups.length === 0" description="暂无群组" />
        </div>
      </el-tab-pane>
      
      <el-tab-pane label="已加入的" name="joined">
        <div class="group-list">
          <div v-for="group in contactStore.joinedGroups" :key="group.groupId" class="group-item">
            <el-avatar :size="48" :src="group.avatar || defaultGroupAvatar" />
            <div class="info">
              <div class="name">{{ group.groupName }}</div>
              <div class="member-count">{{ group.memberCount }} 人</div>
            </div>
            <div class="actions">
              <el-button size="small" @click="startChat(group)">发消息</el-button>
              <el-button size="small" type="danger" @click="handleLeave(group.groupId)">退群</el-button>
            </div>
          </div>
          <el-empty v-if="contactStore.joinedGroups.length === 0" description="暂无群组" />
        </div>
      </el-tab-pane>
    </el-tabs>
    
    <!-- 创建群组对话框 -->
    <el-dialog v-model="showCreateDialog" title="创建群组" width="400px">
      <el-form :model="createForm" :rules="createRules" ref="createFormRef">
        <el-form-item prop="groupName">
          <el-input v-model="createForm.groupName" placeholder="群名称" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" @click="handleCreateGroup">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import { useContactStore } from '@/stores/contact'
import { useChatStore } from '@/stores/chat'
import type { Group } from '@/types'

const router = useRouter()
const contactStore = useContactStore()
const chatStore = useChatStore()

const searchKey = ref('')
const activeTab = ref('owned')
const showCreateDialog = ref(false)
const defaultGroupAvatar = 'https://cube.elemecdn.com/6/86/5945665a3924b3a92285dce84ea8dpng.png'

const createForm = reactive({ groupName: '' })
const createRules = { groupName: [{ required: true, message: '请输入群名称', trigger: 'blur' }] }
const createFormRef = ref()

onMounted(() => {
  contactStore.loadMyGroups()
})

async function startChat(group: Group) {
  await chatStore.openSession(group.groupId, 2)
  router.push('/home/conversations')
}

async function handleCreateGroup() {
  await createFormRef.value?.validate()
  await contactStore.createGroup(createForm.groupName, [])
  ElMessage.success('创建成功')
  showCreateDialog.value = false
}

async function handleLeave(groupId: number) {
  await ElMessageBox.confirm('确定退出该群组吗？', '提示')
  await contactStore.leaveGroup(groupId)
  ElMessage.success('已退群')
}
</script>

<style scoped lang="scss">
.groups-page {
  height: 100%;
  background: #fff;
  display: flex;
  flex-direction: column;
}

.header {
  padding: 15px 20px;
  border-bottom: 1px solid #e8e8e8;
  display: flex;
  gap: 10px;
  
  :deep(.el-input) {
    flex: 1;
  }
}

:deep(.el-tabs__content) {
  flex: 1;
  overflow-y: auto;
}

.group-list {
  padding: 10px 20px;
}

.group-item {
  display: flex;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid #f0f0f0;
  
  .info {
    flex: 1;
    margin-left: 12px;
    
    .name {
      font-size: 14px;
      font-weight: 500;
    }
    
    .member-count {
      font-size: 12px;
      color: #999;
      margin-top: 4px;
    }
  }
  
  .actions {
    display: flex;
    gap: 8px;
  }
}
</style>
```

---

### Task 17: 创建个人中心页面

**Files:**
- Create: `chat-web/src/views/profile/index.vue`

**Step 1: 创建个人中心页面**

```vue
<template>
  <div class="profile-page">
    <div class="profile-header">
      <el-avatar :size="80" :src="userStore.userInfo?.avatar || defaultAvatar" @click="showAvatarDialog = true" />
      <div class="name">{{ userStore.userInfo?.nickname }}</div>
    </div>
    
    <div class="profile-content">
      <el-form :model="form" label-width="80px">
        <el-form-item label="昵称">
          <el-input v-model="form.nickname" />
        </el-form-item>
        <el-form-item label="手机号">
          <el-input v-model="form.phone" disabled />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="form.email" />
        </el-form-item>
        <el-form-item label="生日">
          <el-date-picker v-model="form.birthday" type="date" placeholder="选择日期" />
        </el-form-item>
        <el-form-item label="性别">
          <el-radio-group v-model="form.gender">
            <el-radio :label="0">未知</el-radio>
            <el-radio :label="1">男</el-radio>
            <el-radio :label="2">女</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSave" :loading="saving">保存</el-button>
        </el-form-item>
      </el-form>
    </div>
    
    <div class="profile-footer">
      <el-button type="danger" @click="handleLogout" :loading="loggingOut">退出登录</el-button>
    </div>
    
    <!-- 头像上传对话框 -->
    <el-dialog v-model="showAvatarDialog" title="更换头像" width="400px">
      <el-upload 
        :auto-upload="false" 
        :show-file-list="false"
        :on-change="handleAvatarChange"
        accept="image/*"
      >
        <el-button>选择图片</el-button>
      </el-upload>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import * as userApi from '@/api/user'
import type { UploadFile } from 'element-plus'

const router = useRouter()
const userStore = useUserStore()

const defaultAvatar = 'https://cube.elemecdn.com/3/7c/3ea6beec64369c2642b92c6726f1epng.png'

const form = reactive({
  nickname: '',
  phone: '',
  email: '',
  birthday: '',
  gender: 0
})

const saving = ref(false)
const loggingOut = ref(false)
const showAvatarDialog = ref(false)

onMounted(() => {
  if (userStore.userInfo) {
    Object.assign(form, userStore.userInfo)
  }
})

async function handleSave() {
  saving.value = true
  try {
    await userStore.updateUserInfo(form)
    ElMessage.success('保存成功')
  } catch (e) {
    ElMessage.error('保存失败')
  } finally {
    saving.value = false
  }
}

async function handleAvatarChange(file: UploadFile) {
  if (!file.raw) return
  const { url } = await userApi.uploadAvatar(file.raw)
  await userStore.updateUserInfo({ avatar: url })
  ElMessage.success('头像更新成功')
  showAvatarDialog.value = false
}

async function handleLogout() {
  await ElMessageBox.confirm('确定退出登录吗？', '提示')
  loggingOut.value = true
  userStore.logout()
  router.push('/login')
}
</script>

<style scoped lang="scss">
.profile-page {
  height: 100%;
  background: #fff;
  display: flex;
  flex-direction: column;
}

.profile-header {
  padding: 40px 20px;
  text-align: center;
  
  .name {
    margin-top: 15px;
    font-size: 18px;
    font-weight: 500;
  }
}

.profile-content {
  flex: 1;
  padding: 20px;
}

.profile-footer {
  padding: 20px;
  text-align: center;
  border-top: 1px solid #e8e8e8;
}
</style>
```

---

### Task 18: 编译验证

**Step 1: 安装依赖并编译**

```bash
cd chat-web
npm install
npm run build
```

**Step 2: 启动开发服务器**

```bash
npm run dev
```

---

**Plan complete and saved to `docs/plans/2026-02-25-kamachat-frontend-implementation.md`**

---

## 执行选项

请选择执行方式：

1. **Subagent-Driven (本会话)** - 我为每个任务分配子代理，任务间进行代码审查，快速迭代
2. **Parallel Session (单独会话)** - 在新会话中使用 executing-plans，分批执行并设置检查点

你选择哪种方式？
