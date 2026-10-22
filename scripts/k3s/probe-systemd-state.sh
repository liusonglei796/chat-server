#!/usr/bin/env bash
# 判断 WSL 的关机流程是否处于「已开始但未完成」的卡死状态
set -uo pipefail

echo "==> PID 1"
ps -p 1 -o pid,comm,args --no-headers 2>/dev/null || echo "  (PID1 不存在！)"

echo
echo "==> systemd 运行状态"
systemctl is-system-running 2>&1 || true

echo
echo "==> 是否有关机任务在队列中"
systemctl list-jobs --no-pager 2>&1 | head -15

echo
echo "==> poweroff/shutdown 单元状态"
systemctl is-active systemd-poweroff.service 2>&1 || true
systemctl is-active shutdown.target 2>&1 || true
systemctl is-active poweroff.target 2>&1 || true

echo
echo "==> 最近的关机/启动记录"
journalctl --no-pager -o short-iso 2>/dev/null \
  | grep -E 'poweroff|shutdown.target|Reached target (multi-user|sysinit|poweroff)' | tail -12

echo
echo "==> k3s 单元"
systemctl show k3s -p NRestarts -p ActiveState -p SubState -p ActiveEnterTimestamp -p ExecMainStatus 2>&1

echo
echo "==> 容器运行时"
ls -la /run/k3s/containerd/containerd.sock 2>&1
systemctl is-active k3s 2>&1
