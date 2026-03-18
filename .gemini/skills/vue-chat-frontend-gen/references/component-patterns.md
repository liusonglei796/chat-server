# Vue Component Patterns for Chat Server

## 1. Authentication Component (Login.vue)

```vue
<script setup lang="ts">
import { ref } from 'vue';
import { useAuthStore } from '../stores/auth';

const username = ref('');
const password = ref('');
const auth = useAuthStore();

const handleLogin = async () => {
  try {
    await auth.login({ username: username.value, password: password.value });
    // Redirect to chat
  } catch (err) {
    console.error('Login failed', err);
  }
};
</script>

<template>
  <div class="auth-container">
    <h2>Login</h2>
    <form @submit.prevent="handleLogin">
      <input v-model="username" placeholder="Username" required />
      <input v-model="password" type="password" placeholder="Password" required />
      <button type="submit">Login</button>
    </form>
  </div>
</template>

<style scoped>
.auth-container {
  display: flex;
  flex-direction: column;
  width: 300px;
  margin: auto;
  padding: 2rem;
  border: 1px solid var(--border-color);
}
</style>
```

## 2. Message List Component (MessageList.vue)

```vue
<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useMessageStore } from '../stores/message';

const props = defineProps<{ sessionId: string }>();
const messageStore = useMessageStore();

onMounted(() => {
  messageStore.fetchMessages(props.sessionId);
});
</script>

<template>
  <div class="message-list">
    <div v-for="msg in messageStore.messages" :key="msg.id" class="message-item">
      <span class="sender">{{ msg.senderName }}</span>
      <p class="content">{{ msg.content }}</p>
    </div>
  </div>
</template>

<style scoped>
.message-list {
  flex: 1;
  overflow-y: auto;
  padding: 1rem;
}
</style>
```

## 3. WebSocket Composable (useChatSocket.ts)

```typescript
import { ref } from 'vue';

export function useChatSocket(url: string) {
  const socket = ref<WebSocket | null>(null);
  const isConnected = ref(false);

  const connect = (token: string) => {
    socket.value = new WebSocket(`${url}?token=${token}`);
    
    socket.value.onopen = () => {
      isConnected.value = true;
      console.log('WS Connected');
    };

    socket.value.onmessage = (event) => {
      const data = JSON.parse(event.data);
      // Handle incoming message
    };

    socket.value.onclose = () => {
      isConnected.value = false;
      // Handle reconnect
    };
  };

  return { connect, isConnected };
}
```
