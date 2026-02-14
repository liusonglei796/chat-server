# OpenCode + Oh-My-OpenCode 使用指南

## 目录

- [简介](#简介)
- [安装](#安装)
- [配置](#配置)
- [基础使用](#基础使用)
- [Oh-My-OpenCode 高级功能](#oh-my-opencode-高级功能)
- [常用命令](#常用命令)
- [最佳实践](#最佳实践)
- [故障排查](#故障排查)

---

## 简介

### OpenCode

OpenCode 是一个开源的 AI 编码助手 CLI 工具，支持 75+ LLM 提供商，包括：
- Anthropic Claude
- OpenAI GPT
- Google Gemini
- 本地模型（通过 Ollama）
- GitHub Copilot

### Oh-My-OpenCode

Oh-My-OpenCode 是 OpenCode 的增强插件，提供：
- **Sisyphus 主代理** - 智能任务编排代理
- **多模型协作团队** - Oracle、Librarian、Explore 等专业代理
- **后台并行任务** - 类似真实开发团队的协作模式
- **LSP/AST 工具** - 精确的代码重构和搜索
- **Todo 强制执行** - 确保任务完成
- **Claude Code 兼容** - 完整的兼容层

---

## 安装

### 1. 安装 OpenCode

#### Linux/macOS

```bash
# 下载最新版本
curl -sL "https://github.com/anomalyco/opencode/releases/download/v1.1.47/opencode-linux-x64.tar.gz" -o opencode.tar.gz
tar -xzf opencode.tar.gz
chmod +x opencode

# 移动到系统路径
sudo mv opencode /usr/local/bin/

# 或移动到用户目录
mkdir -p ~/bin
mv opencode ~/bin/
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

#### 验证安装

```bash
opencode --version
# 输出: 1.1.47
```

### 2. 安装 Oh-My-OpenCode

```bash
# 使用 Bun（推荐）
bunx oh-my-opencode install

# 或使用 npm
npx oh-my-opencode install

# 无交互模式安装
npx oh-my-opencode install --no-tui --claude=no --openai=no --gemini=no
```

### 3. 配置提供商

根据您的订阅情况配置 AI 提供商：

```bash
# 启动 OpenCode
opencode

# 在 TUI 中运行
/connect
```

选择您的提供商并完成 OAuth 认证流程。

---

## 配置

### OpenCode 配置文件

配置文件位置（优先级从低到高）：
1. `~/.config/opencode/opencode.json` - 全局配置
2. `opencode.json` - 项目配置
3. `OPENCODE_CONFIG` 环境变量 - 自定义路径

### 基础配置示例

```json
{
  "$schema": "https://opencode.ai/config.json",
  "theme": "opencode",
  "model": "anthropic/claude-sonnet-4-5",
  "small_model": "anthropic/claude-haiku-4-5",
  "autoupdate": true,
  "plugin": ["oh-my-opencode@latest"],
  "server": {
    "port": 4096,
    "hostname": "0.0.0.0",
    "mdns": true
  },
  "permission": {
    "edit": "ask",
    "bash": "ask"
  },
  "formatter": {
    "prettier": { "disabled": false }
  }
}
```

### Oh-My-OpenCode 配置

配置文件位置：`~/.config/opencode/oh-my-opencode.json`

```json
{
  "$schema": "https://raw.githubusercontent.com/code-yeongyu/oh-my-opencode/master/assets/oh-my-opencode.schema.json",
  "agents": {
    "sisyphus": {
      "model": "anthropic/claude-opus-4-5"
    },
    "oracle": {
      "model": "openai/gpt-5.2"
    },
    "librarian": {
      "model": "anthropic/claude-sonnet-4-5"
    },
    "explore": {
      "model": "grok/grok-code"
    },
    "multimodal-looker": {
      "model": "google/gemini-3-pro"
    }
  }
}
```

---

## 基础使用

### 启动 OpenCode

```bash
# 基础启动
opencode

# 指定配置文件
opencode --config /path/to/config.json

# 在指定目录启动
opencode --directory /path/to/project
```

### 基础命令

在 OpenCode TUI 中可用以下命令：

#### `/help` - 帮助

显示所有可用命令和帮助信息。

#### `/status` - 状态

显示当前会话状态、连接的提供商等信息。

#### `/model` - 切换模型

```bash
/model
# 交互式选择模型

/model anthropic/claude-opus-4-5
# 直接指定模型
```

#### `/agent` - 切换代理

```bash
/agent
# 列出所有可用代理

/agent sisyphus
# 切换到 Sisyphus 代理

/agent plan
# 切换到规划代理
```

#### `/share` - 分享会话

```bash
/share
# 分享当前会话

/share --auto
# 自动分享
```

### 快捷键

| 快捷键 | 功能 |
|--------|------|
| `Ctrl+C` | 中止当前操作 |
| `Ctrl+L` | 清屏 |
| `Ctrl+R` | 搜索历史命令 |
| `Tab` | 进入规划模式 |
| `Ctrl+Shift+Esc` | 新建会话 |

---

## Oh-My-OpenCode 高级功能

### 1. Sisyphus 主代理

Sisyphus 是 Oh-My-OpenCode 的核心代理，负责任务编排和执行。

#### 特点

- **智能任务分解** - 自动将复杂任务分解为可执行的步骤
- **多模型协作** - 根据任务类型自动调用合适的子代理
- **后台并行执行** - 同时运行多个后台任务
- **Todo 强制执行** - 确保所有任务完成

#### 使用方式

```bash
# 基础使用
实现一个用户登录功能，包括 JWT 认证和 Redis 缓存

# 使用 ultrawork 模式（推荐）
ulw: 实现一个用户登录功能，包括 JWT 认证和 Redis 缓存

# 使用 ultrathink 模式（深度思考）
ultrathink: 优化现有代码的性能
```

### 2. 专业代理团队

#### Oracle - 架构与调试

负责系统架构设计、复杂问题调试。

```bash
/agent oracle
分析当前代码的架构问题并提出改进建议
```

#### Librarian - 文档与搜索

负责文档查询、代码库搜索、官方文档阅读。

```bash
/agent librarian
查找项目中所有与用户认证相关的代码
```

#### Explore - 快速探索

基于 Contextual Grep 的快速代码库探索。

```bash
/agent explore
查找所有使用了 GORM 的数据库查询代码
```

#### Multimodal Looker - 多模态分析

处理图像、截图等多模态输入。

```bash
/agent multimodal-looker
分析这个 UI 设计图并实现对应的组件
```

### 3. Prometheus 规划器

按 `Tab` 键进入规划模式，创建详细的工作计划。

```bash
# 进入规划模式
Tab

# 创建计划
创建一个 RESTful API，包括：
1. 用户注册
2. 用户登录
3. Token 刷新
4. 用户信息更新

/start-work
# 执行计划
```

### 4. 后台任务

Sisyphus 会自动创建后台任务来加速工作流程：

```bash
# 示例：在实现新功能时
实现一个文件上传功能

# Sisyphus 会自动：
# 1. 创建后台任务搜索现有上传代码
# 2. 创建后台任务查看相关文档
# 3. 创建后台任务检查依赖项
# 4. 主代理专注于实现核心逻辑
```

### 5. LSP/AST 工具

利用语言服务器协议进行精确的代码操作：

```bash
# 重构：重命名变量
将所有 userId 变量重命名为 userID

# 重构：提取函数
将这段重复代码提取为一个单独的函数

# 搜索：AST 模式匹配
查找所有使用了 GORM Where 方法的查询
```

### 6. Todo 强制执行

确保任务完整完成：

```bash
# 如果代理中途停止，系统会：
# 1. 检测未完成的 todo 项
# 2. 自动重新启动代理
# 3. 继续完成剩余任务
```

---

## 常用命令

### 代理管理

```bash
# 列出所有代理
/agent list

# 切换代理
/agent sisyphus

# 查看代理信息
/agent info sisyphus
```

### 会话管理

```bash
# 新建会话
Ctrl+Shift+Esc

# 切换会话
/session list
/session select <session-id>

# 删除会话
/session delete <session-id>

# 归档会话
/session archive <session-id>
```

### 项目管理

```bash
# 切换项目
/project list
/project select <project-id>

# 创建项目
/project create /path/to/project

# 设置项目图标
/project set-icon /path/to/icon.png
```

### 工具操作

```bash
# 运行 shell 命令
bash: ls -la

# 读取文件
read: internal/handler/user_handler.go

# 写入文件
write: new_file.go

# 编辑文件
edit: existing_file.go
```

---

## 最佳实践

### 1. 使用 Ultrawork 模式

对于复杂任务，使用 `ultrawork` 或 `ulw` 关键词：

```bash
ulw: 重构整个用户模块，添加缓存层和错误处理
```

### 2. 明确任务描述

提供清晰、具体的任务描述：

```bash
# ❌ 不好的描述
修复登录功能

# ✅ 好的描述
修复登录功能：当密码错误超过 5 次时，锁定账户 30 分钟，并发送邮件通知
```

### 3. 使用规划模式

对于大型项目，使用 Prometheus 规划器：

```bash
Tab
# 创建详细的实现计划
/start-work
# 执行计划
```

### 4. 利用后台任务

让代理自动创建后台任务来加速工作：

```bash
实现一个复杂的 API 端点
# Sisyphus 会自动创建多个后台任务并行处理
```

### 5. 定期使用 Oracle 进行代码审查

```bash
/agent oracle
审查当前代码的质量、安全性和性能
```

### 6. 使用 Librarian 查找文档

```bash
/agent librarian
查找 GORM 的最佳实践文档
```

### 7. 配置自定义代理

在 `oh-my-opencode.json` 中配置自己的代理：

```json
{
  "agents": {
    "my-specialist": {
      "description": "专门处理支付相关任务",
      "model": "anthropic/claude-opus-4-5",
      "prompt": "你是一个支付系统专家..."
    }
  }
}
```

### 8. 使用自定义命令

在 `opencode.json` 中定义常用命令：

```json
{
  "command": {
    "test": {
      "template": "运行完整的测试套件并生成覆盖率报告",
      "description": "运行测试",
      "agent": "build"
    },
    "deploy": {
      "template": "部署应用到生产环境",
      "description": "部署应用"
    }
  }
}
```

使用：
```bash
/test
/deploy
```

---

## 故障排查

### 问题 1: OpenCode 无法启动

**症状**: 运行 `opencode` 命令无响应或报错

**解决方案**:

```bash
# 检查版本
opencode --version

# 检查配置文件
cat ~/.config/opencode/opencode.json

# 清除缓存
rm -rf ~/.local/share/io.github.clash-verge-rev.clash-verge-rev/

# 重新安装
bunx oh-my-opencode install --force
```

### 问题 2: 提供商连接失败

**症状**: 无法连接到 AI 提供商

**解决方案**:

```bash
# 重新认证
opencode
/connect
# 选择提供商并完成认证

# 检查网络连接
ping api.anthropic.com

# 检查代理设置
export HTTP_PROXY=http://127.0.0.1:7890
export HTTPS_PROXY=http://127.0.0.1:7890
```

### 问题 3: Oh-My-OpenCode 插件未加载

**症状**: 插件功能不可用

**解决方案**:

```bash
# 检查配置
cat ~/.config/opencode/opencode.json
# 确认包含 "plugin": ["oh-my-opencode@latest"]

# 重新安装
npx oh-my-opencode install --force

# 检查插件配置
cat ~/.config/opencode/oh-my-opencode.json
```

### 问题 4: 性能问题

**症状**: 响应缓慢

**解决方案**:

```bash
# 切换到更快的模型
/model anthropic/claude-haiku-4-5

# 减少上下文
/context clear

# 禁用某些功能（在配置中）
{
  "oh-my-opencode": {
    "disabled_hooks": ["pre-tool-use"]
  }
}
```

### 问题 5: Todo 未完成

**症状**: 任务中途停止

**解决方案**:

```bash
# 手动继续
/continue

# 查看待办事项
/todo list

# 强制完成特定任务
/todo complete <task-id>
```

### 问题 6: SSL 证书验证错误

**症状**: `UNKNOWN_CERTIFICATE_VERIFICATION_ERROR` 或 `unknown certificate verification error`

**原因**: 由于代理或网络环境导致 SSL 证书验证失败

**解决方案**:

```bash
# 添加环境变量到 ~/.zshrc（适用于 zsh）
echo 'export NODE_TLS_REJECT_UNAUTHORIZED=0' >> ~/.zshrc

# 添加环境变量到 ~/.bashrc（适用于 bash）
echo 'export NODE_TLS_REJECT_UNAUTHORIZED=0' >> ~/.bashrc

# 重新加载配置
source ~/.zshrc  # 或 source ~/.bashrc
```

或者临时禁用（仅当前会话有效）：

```bash
export NODE_TLS_REJECT_UNAUTHORIZED=0
opencode
```

---

## 进阶技巧

### 1. 多模型协作

利用不同模型的优势：

```bash
# 使用 Opus 4.5 进行架构设计
/agent oracle
设计一个微服务架构

# 使用 GPT-5.2 进行代码生成
/agent sisyphus
实现上述架构

# 使用 Grok 进行代码审查
/agent oracle
审查实现的代码
```

### 2. 工作流自动化

创建自定义工作流：

```json
{
  "command": {
    "full-feature": {
      "template": "ulw: 实现完整功能，包括代码、测试、文档和部署配置",
      "description": "完整功能开发"
    }
  }
}
```

### 3. 与 Git 集成

```bash
# 自动提交
bash: git add . && git commit -m "feat: 新功能"

# 查看变更
bash: git diff

# 创建分支
bash: git checkout -b feature/new-feature
```

### 4. 使用 MCP 服务器

Oh-My-OpenCode 内置多个 MCP 服务器：

- **Exa** - 网络搜索
- **Context7** - 官方文档
- **Grep.app** - GitHub 代码搜索

```bash
# 使用网络搜索
搜索最新的 Go 最佳实践

# 查看官方文档
查看 Gin 框架的官方文档

# GitHub 代码搜索
搜索 GitHub 上的 GORM 使用示例
```

---

## 参考资料

### 官方文档

- [OpenCode 官方文档](https://opencode.ai/docs)
- [Oh-My-OpenCode GitHub](https://github.com/code-yeongyu/oh-my-opencode)
- [OpenCode 配置 Schema](https://opencode.ai/config.json)

### 社区

- [OpenCode Discord](https://discord.gg/opencode)
- [Oh-My-OpenCode Reddit](https://reddit.com/r/opencodeCLI)

### 教程

- [OpenCode 入门教程](https://opencode.ai/docs/intro)
- [Oh-My-OpenCode 概览](https://raw.githubusercontent.com/code-yeongyu/oh-my-opencode/master/docs/guide/overview.md)

---

## 许可证

OpenCode - GPL-3.0 License  
Oh-My-OpenCode - MIT License

---

## 贡献

欢迎提交 Issue 和 Pull Request！

---

<p align="center">
  <b>Happy Coding with OpenCode + Oh-My-OpenCode! 🚀</b>
</p>