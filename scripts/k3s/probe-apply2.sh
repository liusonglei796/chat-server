#!/usr/bin/env bash
# 诊断好友申请链路
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

echo "==> apply-service 最近日志"
kubectl -n "$NS" logs deployment/apply-service --tail=30 2>&1 | tail -30

echo
echo "==> chat_apply.contact_apply 内容"
kubectl -n "$NS" exec mysql-0 -- mysql -uroot -proot123456 -e \
  "SELECT uuid, applicant_id, target_id, status, version, created_at FROM chat_apply.contact_apply ORDER BY id DESC LIMIT 5" 2>&1

echo
echo "==> chat_apply.outbox 内容"
kubectl -n "$NS" exec mysql-0 -- mysql -uroot -proot123456 -e \
  "SELECT event_type, payload, created_at FROM chat_apply.outbox ORDER BY created_at DESC LIMIT 5" 2>&1

echo
echo "==> friendship_service 在 etcd 的注册"
kubectl -n "$NS" exec statefulset/etcd -- etcdctl get / --prefix --keys-only 2>&1 | head -20

echo
echo "==> CDC 日志尾部"
kubectl -n "$NS" logs deployment/cdc-service --tail=15 2>&1 | tail -15
