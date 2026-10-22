#!/usr/bin/env bash
# 清理 local-path-provisioner 在宿主机上留下的 PVC 目录
# 目录名是 PVC 的 UUID，不带业务名，需要按「命名空间已不存在」来判断归属。
set -uo pipefail

export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
BASE=/var/lib/rancher/k3s/storage/pvc

echo "==> 当前 PVC 目录"
ls -1 "$BASE" 2>/dev/null || echo "    (无)"

echo "==> 仍存在的 PVC 映射"
kubectl get pvc -A -o custom-columns='NS:.metadata.namespace,NAME:.metadata.name,PV:.spec.volumeName' --no-headers 2>/dev/null || true

echo "==> 删除所有 PVC 目录（本地验证环境，每次重置都从零开始）"
if [ -d "$BASE" ]; then
  for d in "$BASE"/*; do
    [ -d "$d" ] || continue
    echo "    rm -rf $d"
    rm -rf "$d"
  done
fi

echo "==> 清理后"
ls -1 "$BASE" 2>/dev/null || echo "    (空)"
echo "PVC_CLEAN_OK"
