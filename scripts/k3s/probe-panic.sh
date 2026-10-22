#!/usr/bin/env bash
# 抓 apply-service 的 panic 头部
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

echo "==> apply-service 完整日志中的 panic 段"
kubectl -n "$NS" logs deployment/apply-service 2>&1 | grep -n -B4 -A18 'panic\|goroutine 1 \|runtime error' | head -60

echo
echo "==> apply-service 日志最后 40 行"
kubectl -n "$NS" logs deployment/apply-service --tail=40 2>&1 | tail -40

echo
echo "==> apply-service 启动日志（前 30 行，看 MySQL 初始化）"
kubectl -n "$NS" logs deployment/apply-service 2>&1 | head -30
