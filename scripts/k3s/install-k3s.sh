#!/usr/bin/env bash
# 在 WSL2 Ubuntu 中安装 k3s（本地验证环境专用）
#
# 关键选项说明：
#   --disable traefik        本地验证不需要 Ingress，Q18 已定用 port-forward 暴露
#   --disable servicelb      同上，避免 LoadBalancer 在 WSL2 里占端口
#   --disable metrics-server 无需 HPA/Prometheus
#   --flannel-backend=host-gw 单节点集群用 host-gw，避免 WSL2 内核缺少 vxlan 模块
set -euo pipefail

K3S_VERSION="${K3S_VERSION:-v1.36.5+k3s1}"
INSTALL_ARGS="--disable traefik --disable servicelb --disable metrics-server --flannel-backend=host-gw"

echo "==> 安装 k3s ${K3S_VERSION}"
curl -sfL https://get.k3s.io | \
  INSTALL_K3S_VERSION="${K3S_VERSION}" \
  INSTALL_K3S_EXEC="${INSTALL_ARGS}" \
  sh -

echo "==> 等待节点就绪"
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
for i in $(seq 1 60); do
  if kubectl get nodes >/dev/null 2>&1; then
    kubectl get nodes -o wide
    break
  fi
  sleep 2
done

echo "==> 组件状态"
kubectl get pods -A
echo "==> K3S_INSTALL_OK"
