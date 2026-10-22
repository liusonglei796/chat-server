# scripts/k3s —— 本地 k3s 混沌验证环境

在 **WSL2 + k3s** 上把 chat-server 全量起起来，用 **Toxiproxy** 注入真实弱网流量，
以黑盒方式验证好友拉黑接口（`PUT /friends/block`）的正确性。

> 背景：本机没有 Docker，k3s 跑在 WSL2 里，镜像构建走
> 「宿主机交叉编译 linux/amd64 二进制 + WSL 内 nerdctl 打包进 k3s 自带的 containerd」。

## 一次性准备

```powershell
# 1) 让 WSL2 常驻（关键！见下方「WSL2 反复关机」）
pwsh -File scripts/k3s/wsl-keepalive.ps1

# 2) 依赖：k3s / nerdctl+buildkit / iptables / 镜像加速器
wsl -d Ubuntu-24.04 -u root -- bash /mnt/d/repo/chat-server/scripts/k3s/install-k3s.sh
wsl -d Ubuntu-24.04 -u root -- bash /mnt/d/repo/chat-server/scripts/k3s/install-nerdctl.sh
wsl -d Ubuntu-24.04 -u root -- bash /mnt/d/repo/chat-server/scripts/k3s/install-iptables.sh
wsl -d Ubuntu-24.04 -u root -- bash /mnt/d/repo/chat-server/scripts/k3s/setup-registry-mirror.sh
```

## 日常流程

```powershell
# 构建镜像
pwsh -File scripts/k3s/build-images.ps1          # 全部
pwsh -File scripts/k3s/build-images.ps1 -Services friendship-service   # 单个
pwsh -File scripts/k3s/build-images.ps1 -SkipE2E                     # 不构建 e2e

# 部署（严格有序）
wsl -d Ubuntu-24.04 -u root -- bash /mnt/d/repo/chat-server/scripts/k3s/deploy.sh

# 跑 e2e
wsl -d Ubuntu-24.04 -u root -- bash /mnt/d/repo/chat-server/scripts/k3s/run-e2e.sh

# 清空重来
wsl -d Ubuntu-24.04 -u root -- bash /mnt/d/repo/chat-server/scripts/k3s/reset.sh
wsl -d Ubuntu-24.04 -u root -- bash /mnt/d/repo/chat-server/scripts/k3s/clean-pvc.sh
```

## 脚本清单

| 脚本 | 作用 |
|---|---|
| `wsl-keepalive.ps1` | **必须先跑**。保持一个常驻 wsl.exe 会话，防止 WSL 回收 VM |
| `install-k3s.sh` | 安装 k3s v1.36.5+k3s1（禁 traefik/servicelb/metrics-server，flannel 用 host-gw） |
| `install-nerdctl.sh` | 安装 nerdctl + buildkitd，buildkitd 复用 k3s 的 containerd（k8s.io 命名空间） |
| `install-iptables.sh` | 安装 iptables（k3s 的 flannel 与内嵌 kube-proxy 都需要） |
| `setup-registry-mirror.sh` | 配置镜像加速器（docker.io 的 DNS 在本机会被污染） |
| `build-images.ps1` | 交叉编译 + 打包镜像（含 e2e 测试镜像） |
| `deploy.sh` | 按序部署（顺序本身就是设计，见文件头注释） |
| `run-e2e.sh` | 以 Job 形式在集群内跑 e2e 套件，日志落到 `docs/e2e-run.log` |
| `reset.sh` / `clean-pvc.sh` | 清空业务命名空间与 PVC 数据 |
| `smoke.sh` / `test-envoy-path.sh` | 链路冒烟 |
| `probe-*.sh` | 排障探针（DNS / etcd / 熔断 / 网关 / 租约 …） |

## 踩过的坑（都已在脚本里规避）

1. **WSL2 反复关机** —— 最后一个 `wsl.exe` 会话退出后，WSL 会调用 `systemctl poweroff`；
   而 k3s + containerd 的容器无法在 10 秒内干净退出，WSL 于是升级为硬重启
   （`WSL ERROR: systemctl poweroff did not terminate the instance in 10000 ms, calling reboot(RB_POWER_OFF)`）。
   后果是 k3s 反复重启 → flannel 反复重写 `/run/flannel/subnet.env` →
   所有 Pod 的 sandbox 反复重建（SandboxChanged）→ 集群永远不稳定。
   **解法**：`wsl-keepalive.ps1` 用 WMI（`Win32_Process.Create`）创建一个**脱离进程树**的
   常驻 `sleep infinity` 会话。用 `Start-Process` 创建的会被调用方的进程清理连带杀掉（实测无效）。
2. **docker.io 的 DNS 被污染** —— 解析到不可达 IP，镜像拉取随机失败。
3. **k8s 里 MySQL 的 `command:` 会覆盖 ENTRYPOINT** —— `docker-entrypoint.sh` 不再执行，
   初始化与 initdb 全丢，mysqld 报 `Failed to find valid data directory`。
   k8s 里要用 `args:` 透传参数。
4. **MySQL 的 datadir 一旦初始化中断就会残留半成品** —— 之后只能删 PVC 重建。
5. **命名空间卡在 Terminating** —— `reset.sh` 会直接摘 finalizer 强制删除。

## 部署顺序为什么是那样

`deploy.sh` 的每一步都对应一个「顺序错了就会得出假结论」的坑：

1. **MySQL 先起**，并校验 5 个拆分库 + `svc_cdc` 权限 + `gtid_mode=ON`/`binlog_format=ROW`
   —— 否则各服务的 AutoMigrate 连不上库、CDC 也订阅不到 outbox 表。
2. **Toxiproxy 先于业务服务**，且先跑 `toxiproxy-init` Job 建好代理路由
   —— `friendship_service` 被配置成通过 `toxiproxy:33066` 连 MySQL，代理不存在它起不来。
3. **业务服务先于 CDC** —— CDC 的 canal 在启动时解析表过滤正则 `^chat_.*\.outbox$`，
   表还不存在时它不会订阅，之后表建好了也不补订阅；
   反过来若 CDC 先启动，它冷启动会从「当前 master GTID」开始，跳过历史 binlog。
4. **Kafka 消费者在测试数据产生之前就已订阅** —— 消费者用 `ConsumeResetOffset(AtEnd())`，
   冷启动会跳过已有事件；e2e 里用「申请通过」产生的 `friend_apply_passed` 事件做位点预热。
