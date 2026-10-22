#!/usr/bin/env bash
# 诊断网关 -> apply_service 的 gRPC 调用失败
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

echo "==> chat-server(gateway) 最近日志"
kubectl -n "$NS" logs deployment/chat-server --tail=40 2>&1 | tail -40

echo
echo "==> 网关内解析 apply-service"
kubectl -n "$NS" exec deployment/chat-server -- nslookup apply-service 2>&1 | tail -6

echo
echo "==> 网关内连 apply-service:50053"
kubectl -n "$NS" exec deployment/chat-server -- sh -c 'nc -z -w3 apply-service 50053 && echo TCP_OK || echo TCP_FAIL' 2>&1 | tail -3

echo
echo "==> 网关内连 user-service:50051（对照组）"
kubectl -n "$NS" exec deployment/chat-server -- sh -c 'nc -z -w3 user-service 50051 && echo TCP_OK || echo TCP_FAIL' 2>&1 | tail -3

echo
echo "==> 网关内连 friendship-service:50054（经 toxiproxy 劫持）"
kubectl -n "$NS" exec deployment/chat-server -- sh -c 'nc -z -w3 friendship-service 50054 && echo TCP_OK || echo TCP_FAIL' 2>&1 | tail -3

echo
echo "==> etcd 中 apply_service 的注册值"
kubectl -n "$NS" exec statefulset/etcd -- etcdctl get /microservices/apply_service --prefix 2>&1 | head -8
