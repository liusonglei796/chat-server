#!/usr/bin/env bash
# 列出所有 mysql 容器（按创建时间倒序）并打印最新一个的日志
set -uo pipefail

echo "==> mysql 容器列表"
k3s crictl ps -a --name mysql 2>/dev/null | head -20

echo
echo "==> 最新容器日志"
ID=$(k3s crictl ps -a --name mysql --quiet 2>/dev/null | head -1)
echo "container id: $ID"
if [ -n "$ID" ]; then
  k3s crictl logs "$ID" 2>&1 | tail -40
fi

echo
echo "==> 容器状态"
if [ -n "$ID" ]; then
  k3s crictl inspect "$ID" 2>/dev/null | jq -r '.status | {state, exitCode, reason, message, finishedAt}' 2>/dev/null || true
fi
