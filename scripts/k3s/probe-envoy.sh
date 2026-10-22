#!/usr/bin/env bash
# 在集群内直接诊断 Envoy 路由
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

echo "==> envoy Service / Endpoints"
kubectl -n "$NS" get svc envoy -o wide 2>&1
kubectl -n "$NS" get endpoints envoy 2>&1

echo
echo "==> 从集群内 curl envoy:9080 /auth/login"
kubectl -n "$NS" run tmpcurl --rm -i --restart=Never --image=alpine:3.18 --quiet -- \
  sh -c "wget -q -O- --header='Content-Type: application/json' --post-data='{\"username\":\"x\",\"password\":\"yyyyyy\"}' http://envoy:9080/auth/login" 2>&1 | head -8

echo
echo "==> envoy admin clusters（看 user_service_cluster 的解析结果）"
kubectl -n "$NS" run tmpadm --rm -i --restart=Never --image=alpine:3.18 --quiet -- \
  sh -c "wget -q -O- http://envoy:9901/clusters | head -c 2000" 2>&1 | tr ',' '\n' | grep -i -E 'user_service|friendship|address' | head -20

echo
echo "==> envoy 最近访问日志"
kubectl -n "$NS" logs deployment/envoy --tail=12 2>&1 | tail -12
