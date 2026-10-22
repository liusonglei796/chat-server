#!/usr/bin/env bash
# 在 WSL 内构建单个服务镜像。
#
# 关键点：构建上下文放在 WSL 本地盘（/root/k3s-build），而不是 /mnt/d。
# /mnt/d 是 9p 挂载，读写 Windows 文件极慢；把 8 个二进制（约 370MB）整个作为
# 构建上下文传给 buildkit，每次要几十秒到几分钟。
# 这里只把「当前要打的那一个二进制」拷进上下文，且落在 WSL 的 ext4 上。
#
# 用法：build-image.sh <binary-name> <image-name> [dockerfile]
set -euo pipefail

BIN="${1:?需要指定二进制名}"
IMG="${2:?需要指定镜像名}"
DOCKERFILE="${3:-Dockerfile.service}"
REPO=/mnt/d/repo/chat-server
CTX=/root/k3s-build

mkdir -p "$CTX/bin"
cp -f "$REPO/build/k3s/bin/$BIN" "$CTX/bin/app"
cp -f "$REPO/build/k3s/$DOCKERFILE" "$CTX/$DOCKERFILE"
rm -rf "$CTX/web"
cp -r "$REPO/web" "$CTX/web"

cd "$CTX"
k3s-nerdctl build \
  --build-arg SERVICE_BINARY=app \
  --build-arg TEST_BINARY=app \
  -t "$IMG" \
  -f "$DOCKERFILE" .
echo "BUILD_IMAGE_OK $IMG"
