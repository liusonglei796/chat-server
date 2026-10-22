#!/usr/bin/env bash
# 探测可用的 nerdctl full 版本（GitHub Release 资产名不含 'v' 前缀）
for v in 2.1.1 2.1.0 2.0.0 1.7.7 1.7.6; do
  url="https://github.com/containerd/nerdctl/releases/download/v${v}/nerdctl-full-${v}-linux-amd64.tar.gz"
  code=$(curl -s -o /dev/null -w '%{http_code}' -L --max-time 25 "$url")
  echo "$v -> $code"
done
