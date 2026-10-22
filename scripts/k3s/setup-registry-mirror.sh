#!/usr/bin/env bash
# 配置容器镜像加速器
#
# 背景：本机网络对 registry-1.docker.io 的 DNS 解析会被劫持到不可达 IP，
# 导致 docker.io 镜像拉取随机失败（表现为 ImagePullBackOff）。
# k3s 的 registries.yaml 同时作用于 kubelet 与 buildkitd 的 containerd worker，
# 一处配置两条链路都生效。
set -euo pipefail

mkdir -p /etc/rancher/k3s

cat > /etc/rancher/k3s/registries.yaml <<'EOF'
mirrors:
  "docker.io":
    endpoint:
      - "https://docker.m.daocloud.io"
      - "https://docker.1ms.run"
      - "https://docker.xuanyuan.me"
  "ghcr.io":
    endpoint:
      - "https://docker.m.daocloud.io"
EOF

echo "==> 已写入 /etc/rancher/k3s/registries.yaml"
cat /etc/rancher/k3s/registries.yaml

echo "==> 重启 k3s 使配置生效"
systemctl restart k3s
sleep 20

echo "==> 验证拉取"
k3s-nerdctl pull alpine:3.18
echo "REGISTRY_MIRROR_OK"
