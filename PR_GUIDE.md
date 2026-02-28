# GitHub Pull Request 操作指南

## 方式一：网页端直接操作（推荐新手）

### 1. Fork 别人的仓库
- 打开目标仓库页面
- 点击右上角 **Fork** 按钮
- 选择你的 GitHub 账号，创建 fork

### 2. 克隆到本地
```bash
git clone https://github.com/你的用户名/仓库名.git
cd 仓库名
```

### 3. 创建分支
```bash
git checkout -b feature/你的功能名
```

### 4. 修改代码并提交
```bash
git add .
git commit -m "描述你的修改"
```

### 5. 推送到你的 fork
```bash
git push origin feature/你的功能名
```

### 6. 创建 PR
- 刷新你的仓库页面
- 会看到 **Compare & pull request** 按钮，点击
- 填写标题和描述
- 点击 **Create pull request**

---

## 方式二：通过 GitHub CLI

### 1. Fork + Clone 一步到位
```bash
gh repo fork owner/repo --clone
```

### 2. 创建分支、修改、提交
同上方式一的步骤 3-4

### 3. 创建 PR
```bash
gh pr create --title "你的标题" --body "详细描述"
```

---

## 方式三：保持与上游同步

### 添加上游 remote
```bash
git remote add upstream https://github.com/原作者/仓库名.git
```

### 同步上游更新
```bash
git fetch upstream
git checkout main
git merge upstream/main
```

### 推送到你自己的 fork
```bash
git push origin main
```

---

## 注意事项

1. **PR 描述要清晰**：说明改了什么、为什么改、怎么测试
2. **保持分支干净**：每次功能开新分支，不要在 main 上直接改
3. **及时同步上游**：提交 PR 前先拉取最新代码，避免冲突
4. **遵循项目规范**：查看项目的 CONTRIBUTING.md（如果有）
