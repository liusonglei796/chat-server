#!/usr/bin/env bash
# WSL2 里 k3s 依赖 iptables：
#   - flannel 的 iptables 模式需要 iptables-save/iptables-restore
#   - 内嵌 kube-proxy 需要 iptables 二进制来写 Service/ClusterIP 规则
# 缺了它们，Pod 能拿到 IP 但 Service 访问不通。
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

echo "==> apt update"
apt-get update -qq

echo "==> 安装 iptables / iproute2 / conntrack 等依赖"
apt-get install -y -qq --no-install-recommends \
  iptables \
  iproute2 \
  iputils-ping \
  conntrack \
  socat \
  netcat-openbsd \
  ca-certificates \
  curl \
  jq \
  >/dev/null

echo "==> 校验"
which iptables iptables-save iptables-restore ip
iptables --version
echo "IPTABLES_OK"
