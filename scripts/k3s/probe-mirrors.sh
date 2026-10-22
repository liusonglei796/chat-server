#!/usr/bin/env bash
# 探测可用的容器镜像加速器
set -uo pipefail

MIRRORS=(
  "https://docker.m.daocloud.io"
  "https://dockerproxy.com"
  "https://mirror.ccs.tencentyun.com"
  "https://hub-mirror.c.163.com"
  "https://docker.1ms.run"
  "https://docker.xuanyuan.me"
)

for m in "${MIRRORS[@]}"; do
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 12 \
    "$m/v2/library/alpine/manifests/3.18" \
    -H 'Accept: application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json' 2>/dev/null)
  echo "$m -> $code"
done

echo "--- 直连 docker.io ---"
code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 12 https://registry-1.docker.io/v2/ 2>/dev/null)
echo "registry-1.docker.io -> $code"

echo "--- DNS ---"
getent hosts registry-1.docker.io || true
