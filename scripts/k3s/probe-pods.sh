#!/usr/bin/env bash
# 汇总 chat-system 里所有非 Running 的 Pod 及其原因
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

echo "==> pods"
kubectl -n "$NS" get pods -o wide

echo
echo "==> 非 Running Pod 的最近事件"
for p in $(kubectl -n "$NS" get pods --no-headers 2>/dev/null | awk '$3!="Running" && $3!="Completed" {print $1}'); do
  echo "--- $p ---"
  kubectl -n "$NS" describe pod "$p" 2>/dev/null | sed -n '/^Events:/,$p' | tail -6
done

echo
echo "==> 容器退出码"
for c in $(k3s crictl ps -a --quiet 2>/dev/null); do
  st=$(k3s crictl inspect "$c" 2>/dev/null)
  name=$(echo "$st" | jq -r '.status.state | keys[0]' 2>/dev/null)
  code=$(echo "$st" | jq -r '.status.exitCode // "-"' 2>/dev/null)
  reason=$(echo "$st" | jq -r '.status.reason // "-"' 2>/dev/null)
  log=$(echo "$st" | jq -r '.info.config.metadata.name // "-"' 2>/dev/null)
  [ "$name" = "exited" ] && echo "  $log exited code=$code reason=$reason"
done | sort -u | head -30

echo
echo "==> k3s 最近 20 条关键日志"
journalctl -u k3s --no-pager -n 600 2>/dev/null | grep -i -E 'kill|stop|evict|error|failed to start' | tail -20
