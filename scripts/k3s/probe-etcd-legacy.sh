#!/usr/bin/env bash
# 用无前导斜杠的前缀查询遗留注册键
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

echo "==> 以 'apply_service/' 前缀查询（命名服务使用的形式）"
kubectl -n "$NS" exec statefulset/etcd -- etcdctl get apply_service/ --prefix --keys-only 2>&1 | head -10

echo
echo "==> 以 'user_service/' 前缀查询"
kubectl -n "$NS" exec statefulset/etcd -- etcdctl get user_service/ --prefix --keys-only 2>&1 | head -10

echo
echo "==> 以 '/' 前缀查询（只列第一层）"
kubectl -n "$NS" exec statefulset/etcd -- etcdctl get / --prefix --keys-only 2>&1 | head -20

echo
echo "==> 租约列表"
kubectl -n "$NS" exec statefulset/etcd -- etcdctl lease list 2>&1 | head -20

echo
echo "==> 命名服务写入的键值内容"
kubectl -n "$NS" exec statefulset/etcd -- etcdctl get apply_service/ --prefix 2>&1 | head -10
