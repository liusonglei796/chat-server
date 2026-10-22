#!/usr/bin/env bash
# 验证 etcd naming service 的前提条件
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

echo "==> etcd 全部键"
kubectl -n "$NS" exec statefulset/etcd -- etcdctl get / --prefix --keys-only 2>&1

echo
echo "==> etcd 中是否存在 health 键（etcd naming service 的硬性前提）"
kubectl -n "$NS" exec statefulset/etcd -- etcdctl get health 2>&1 | head -4

echo
echo "==> apply_service 启动日志中的 legacy 注册告警"
kubectl -n "$NS" logs deployment/apply-service 2>&1 | grep -i -E 'legacy|register|etcd' | head -10

echo
echo "==> friendship_service 启动日志中的 legacy 注册告警"
kubectl -n "$NS" logs deployment/friendship-service 2>&1 | grep -i -E 'legacy|register|etcd' | head -10

echo
echo "==> chat-server 启动日志中的 legacy 注册告警"
kubectl -n "$NS" logs deployment/chat-server 2>&1 | grep -i -E 'legacy|register|etcd' | head -10
