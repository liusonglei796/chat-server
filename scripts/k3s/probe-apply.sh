#!/usr/bin/env bash
# 诊断 apply-service 调用失败 与 toxiproxy gRPC 代理链路
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

echo "==> apply-service 日志"
kubectl -n "$NS" logs deployment/apply-service --tail=25 2>&1 | tail -25

echo
echo "==> friendship-service 日志"
kubectl -n "$NS" logs deployment/friendship-service --tail=15 2>&1 | tail -15

echo
echo "==> toxiproxy 代理列表"
kubectl -n "$NS" exec deployment/toxiproxy -- wget -q -O- http://127.0.0.1:8474/proxies 2>&1 | head -5

echo
echo "==> friendship-service Service 的 endpoint"
kubectl -n "$NS" get endpoints friendship-service friendship-service-real 2>&1

echo
echo "==> 从 apply-service 容器内测试到 friendship_service 的连通性"
kubectl -n "$NS" exec deployment/apply-service -- sh -c 'wget -q -O- http://friendship-service:50054 2>&1 | head -2; echo "exit=$?"' 2>&1 | head -5

echo
echo "==> 从 apply-service 容器内测试 etcd"
kubectl -n "$NS" exec deployment/apply-service -- sh -c 'wget -q -O- http://etcd:2379/version 2>&1 | head -2' 2>&1 | head -3

echo
echo "==> gateway 容器内的 DNS 解析"
kubectl -n "$NS" exec deployment/chat-server -- sh -c 'nslookup friendship-service 2>&1 | tail -5' 2>&1 | tail -6
