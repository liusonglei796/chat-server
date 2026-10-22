#!/usr/bin/env bash
# 抓取 k3s 每次重启前后的日志边界，定位退出原因
set -uo pipefail

echo "==> k3s unit 状态"
systemctl show k3s -p NRestarts -p Result -p ExecMainCode -p ExecMainStatus -p ActiveEnterTimestamp

echo
echo "==> 最近的启动标记"
journalctl -u k3s --no-pager -o short-iso | grep -n -E "Starting k3s|Stopping k3s|Received SIGTERM|panic|fatal|Failed to start|failed to" | tail -30

echo
echo "==> 最后 60 行"
journalctl -u k3s --no-pager -o short-iso | tail -60
