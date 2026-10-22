#!/usr/bin/env bash
# 清空本地 k3s 验证环境（保留集群本身，只删业务命名空间与其数据）
#
# 用途：
#   - MySQL 的 datadir 一旦在初始化过程中被中断（例如首次启动时缺少 iptables），
#     就会残留半成品数据，mysqld 报 "Failed to find valid data directory" 且无法自愈，
#     必须连 PVC 一起删除重建。
set -euo pipefail

export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

log() { echo -e "\n\033[1;36m==> $*\033[0m"; }

log "删除命名空间 ${NS}"
kubectl delete namespace "$NS" --ignore-not-found --wait=true --timeout=120s || true

# 命名空间卡在 Terminating 是本地环境的常见现象（PVC protection finalizer 等不到
# storage controller 处理）。这里直接摘掉 finalizer 强制删除。
for i in $(seq 1 30); do
  if ! kubectl get namespace "$NS" >/dev/null 2>&1; then
    echo "    命名空间已删除"
    break
  fi
  NS_JSON=$(kubectl get namespace "$NS" -o json 2>/dev/null || true)
  if [ -n "$NS_JSON" ]; then
    echo "$NS_JSON" | jq -c '.spec.finalizers = []' | kubectl replace --raw \
      "/api/v1/namespaces/${NS}/finalize" -f - >/dev/null 2>&1 || true
  fi
  sleep 2
done
kubectl get namespace "$NS" >/dev/null 2>&1 && fail "命名空间 ${NS} 无法删除"

log "删除残留的 PVC（local-path 会把它们留在宿主机上）"
for pvc in $(kubectl get pvc -A -o name 2>/dev/null | grep -E "mysql-data|kafka-data|redis-data|etcd-data|chat-static-pvc" || true); do
  ns=$(echo "$pvc" | cut -d/ -f2)
  name=$(echo "$pvc" | cut -d/ -f3)
  echo "    删除 $ns/$name"
  kubectl -n "$ns" delete pvc "$name" --ignore-not-found --wait=true --timeout=120s
done

log "清理 local-path-provisioner 留下的宿主机目录"
for d in /var/lib/rancher/k3s/storage/pvc/*; do
  case "$d" in
    *mysql*|*kafka*|*redis*|*etcd*|*static*)
      echo "    删除 $d"
      rm -rf "$d" || true
      ;;
  esac
done

log "删除 toxiproxy-init Job（下次部署会重建）"
kubectl -n "$NS" delete job toxiproxy-init --ignore-not-found >/dev/null 2>&1 || true

echo -e "\n\033[1;32mRESET_OK\033[0m"
