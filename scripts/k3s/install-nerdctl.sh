#!/usr/bin/env bash
# 在 WSL2 中安装 nerdctl + buildkitd，并让 buildkitd 复用 k3s 自带的 containerd。
#
# 目的：本机没有 Docker，但 k3s 自带 containerd（命名空间 k8s.io）。
# buildkitd 以 containerd worker 模式直接写入 k3s 的 containerd，
# 因此 nerdctl build 产出的镜像 kubelet 可以直接拉起，无需 docker save/load。
set -euo pipefail

# 注意：Release tag 带 v 前缀（v2.1.1），但资产文件名不带（nerdctl-full-2.1.1-...）
NERDCTL_VER="${NERDCTL_VER:-2.1.1}"
CONTAINERD_SOCK="/run/k3s/containerd/containerd.sock"
BUILDKIT_SOCK="unix:///run/buildkit/buildkitd.sock"

echo "==> 下载 nerdctl ${NERDCTL_VER} (full: 含 nerdctl/buildctl/buildkitd)"
cd /tmp
curl -fsSL -o nerdctl.tar.gz \
  "https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VER}/nerdctl-full-${NERDCTL_VER}-linux-amd64.tar.gz"
tar -C /usr/local -xzf nerdctl.tar.gz
rm -f nerdctl.tar.gz

echo "==> 配置 buildkitd 使用 k3s containerd 的 k8s.io 命名空间"
mkdir -p /etc/buildkit
cat > /etc/buildkit/buildkitd.toml <<EOF
[worker.containerd]
  enabled = true
  address = "${CONTAINERD_SOCK}"
  namespace = "k8s.io"

[worker.oci]
  enabled = false
EOF

cat > /etc/systemd/system/buildkit.service <<EOF
[Unit]
Description=BuildKit (containerd worker -> k3s containerd)
After=network.target k3s.service
Requires=k3s.service

[Service]
ExecStart=/usr/local/bin/buildkitd --config /etc/buildkit/buildkitd.toml --addr ${BUILDKIT_SOCK}
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now buildkit
sleep 4

echo "==> buildkitd worker 状态"
buildctl --addr "${BUILDKIT_SOCK}" debug workers

cat > /usr/local/bin/k3s-nerdctl <<EOF
#!/usr/bin/env bash
exec nerdctl --address ${CONTAINERD_SOCK} --namespace k8s.io "\$@"
EOF
chmod +x /usr/local/bin/k3s-nerdctl

echo "==> nerdctl 版本"
k3s-nerdctl --version
echo "==> NERDCTL_INSTALL_OK"
