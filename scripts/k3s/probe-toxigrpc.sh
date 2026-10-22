#!/usr/bin/env bash
# 诊断 toxiproxy gRPC 代理 与 friendship_service
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

echo "==> toxiproxy 代理列表"
kubectl -n "$NS" run tmptoxi --rm -i --restart=Never --image=alpine:3.18 --quiet -- \
  sh -c "wget -q -O- http://toxiproxy:8474/proxies" 2>&1 | head -6

echo
echo "==> friendship-service 日志"
kubectl -n "$NS" logs deployment/friendship-service --tail=20 2>&1 | tail -20

echo
echo "==> 从集群内直连 friendship-service-real:50054（绕过 toxiproxy）"
kubectl -n "$NS" run tmpfs --rm -i --restart=Never --image=alpine:3.18 --quiet -- \
  sh -c "nc -z -w3 friendship-service-real 50054 && echo TCP_REAL_OK || echo TCP_REAL_FAIL; nc -z -w3 friendship-service 50054 && echo TCP_PROXY_OK || echo TCP_PROXY_FAIL" 2>&1 | head -6

echo
echo "==> Envoy 路由表（看每个 route 的 prefix 与 cluster）"
kubectl -n "$NS" logs deployment/envoy --tail=200 2>&1 | grep -o '"path":"[^"]*","[^}]*' | tail -6
