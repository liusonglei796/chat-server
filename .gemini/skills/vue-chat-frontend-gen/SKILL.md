---
name: vue-chat-frontend-gen
description: Generates Vue 3 (Composition API, TypeScript) frontend components and services for the Chat Server backend. Use when you need to build UI for authentication, messaging, group management, and friendship management that integrates with the Go-based chat server.
---

# Vue Chat Frontend Generator

This skill assists in building a modern Vue 3 frontend that interacts with the Chat Server backend.

## Design Principles

- **Framework**: Vue 3 with Composition API (`<script setup>`).
- **Language**: TypeScript for strong typing and DTO mapping.
- **Styling**: Vanilla CSS with CSS Variables for consistent theming.
- **API Client**: Axios for HTTP requests, with centralized interceptors.
- **Real-time**: Native WebSocket integration for chat messages.
- **State Management**: Pinia (or simple reactive stores) for shared state.

## Core Workflows

### 1. DTO to TypeScript Mapping

When implementing a feature, first map the relevant Go DTOs to TypeScript interfaces.

Example mapping for `UserLoginRequest`:
- Go: `type UserLoginRequest struct { Username string; Password string }`
- TS: `interface UserLoginRequest { username: string; password: string; }`

### 2. Service Layer Generation

Create Axios services for each backend module (auth, user, message, etc.).

- Base URL should be configurable (e.g., `import.meta.env.VITE_API_BASE_URL`).
- Include request/response interceptors for JWT token management.

### 3. Component Architecture

- **SFC Structure**: Template, Script Setup, then Style.
- **Props/Emits**: Use `defineProps` and `defineEmits`.
- **Validation**: Use `v-model` and simple reactive validation logic.

### 4. WebSocket Integration

Implement a `useChatSocket` composable to handle:
- Connection establishment and authentication.
- Heartbeats (if required by backend).
- Message routing (dispatching incoming messages to appropriate stores).

## Component Templates

Refer to `references/component-patterns.md` for specific templates:
- Login/Register
- Message List & Chat Input
- Group Member Management
- Friend Request List

## Styling Guidelines

- Use a `base.css` for resets and global variables.
- Component styles should be `scoped`.
- Prefer Flexbox and CSS Grid for layout.
