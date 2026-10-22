#!/usr/bin/env bash
# 检查 k3s containerd 的镜像加速配置（新版 k3s 用 certs.d 目录）
set -uo pipefail

D=/var/lib/rancher/k3s/agent/etc/containerd/certs.d
echo "==> certs.d 目录"
ls -R "$D" 2>&1 | head -30

echo
echo "==> hosts.toml 内容"
find "$D" -name 'hosts.toml' 2>/dev/null | while read -r f; do
  echo "--- $f ---"
  cat "$f"
done

echo
echo "==> registries.yaml"
cat /etc/rancher/k3s/registries.yaml 2>/dev/null

echo
echo "==> 测试加速器能否取到 alpine manifest"
for m in https://docker.m.daocloud.io https://docker.1ms.run https://docker.xuanyuan.me; do
  echo "--- $m ---"
  token=$(curl -s --max-time 15 "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/alpine:pull" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
  if [ -z "$token" ]; then echo "    (取 token 失败)"; continue; fi
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 \
    -H "Authorization: Bearer $token" \
    -H 'Accept: application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json' \
    "$m/v2/library/alpine/manifests/3.18")
  echo "    manifest -> $code"
done
