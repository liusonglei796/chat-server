#!/usr/bin/env bash
# 运行好友拉黑接口的黑盒 e2e 套件（集群内 Job）
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system
REPO_ROOT="${REPO_ROOT:-/mnt/d/repo/chat-server}"
OUT="${OUT:-$REPO_ROOT/docs/e2e-run.log}"

log() { echo -e "\n\033[1;36m==> $*\033[0m"; }

log "清理旧的 e2e Job"
kubectl -n "$NS" delete job e2e --ignore-not-found --wait=true >/dev/null 2>&1 || true

log "提交 e2e Job"
kubectl apply -f "$REPO_ROOT/deploy/k8s/tests/e2e-job.yaml"

log "等待 Job 完成（最多 25 分钟）"
FINISHED=0
for i in $(seq 1 150); do
  STATUS=$(kubectl -n "$NS" get job e2e -o jsonpath='{.status}' 2>/dev/null || echo "")
  if echo "$STATUS" | grep -q '"succeeded"'; then FINISHED=1; break; fi
  if echo "$STATUS" | grep -q '"failed"'; then FINISHED=2; break; fi
  sleep 10
done

log "Job 日志"
kubectl -n "$NS" logs job/e2e --tail=-1 2>&1 | tee "$OUT"

echo
if [ "$FINISHED" = "1" ]; then
  echo -e "\033[1;32mE2E_JOB_SUCCEEDED\033[0m（日志已存到 $OUT）"
elif [ "$FINISHED" = "2" ]; then
  echo -e "\033[1;31mE2E_JOB_FAILED\033[0m（日志已存到 $OUT）"
else
  echo -e "\033[1;31mE2E_JOB_TIMEOUT\033[0m（日志已存到 $OUT）"
  kubectl -n "$NS" describe job e2e 2>&1 | tail -20
fi
