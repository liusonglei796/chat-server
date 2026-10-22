#!/usr/bin/env bash
# 观察 legacy etcd 注册键的生命周期
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

echo "==> 重启 apply-service，并每秒观察 etcd 键"
kubectl -n "$NS" rollout restart deployment/apply-service >/dev/null
for i in $(seq 1 25); do
  KEYS=$(kubectl -n "$NS" exec statefulset/etcd -- etcdctl get / --prefix --keys-only 2>/dev/null | tr -d '\r' | grep -v '^$' | tr '\n' ' ')
  printf "t=%2ds : %s\n" "$i" "$KEYS"
  sleep 1
done
