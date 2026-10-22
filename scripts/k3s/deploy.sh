#!/usr/bin/env bash
# 按严格顺序把 chat-server 部署到本地 k3s。
#
# 顺序不是随便排的，每一步都对应一个「顺序错了就会得出假结论」的坑：
#
#  1) MySQL 必须先带 GTID/binlog 起来并执行 split_db.sql 建出 5 个拆分库，
#     否则各服务的 AutoMigrate 连不上库、CDC 也订阅不到 outbox 表。
#  2) Toxiproxy 必须在业务服务之前就绪并创建好代理路由，
#     因为 friendship_service 被配置成通过 toxiproxy:33066 连 MySQL，
#     代理不存在时它会启动失败。
#  3) 业务服务必须先于 CDC 启动：
#     CDC 的 canal 在启动时解析 IncludeTableRegex ^chat_.*\.outbox$，
#     表还不存在时它不会订阅，之后表建好了也不会补订阅；
#     反过来若 CDC 先启动，它冷启动会从「当前 master GTID」开始，跳过历史 binlog。
#  4) Kafka 消费者（friendship/message/group 的 *_domain_events group）
#     使用 ConsumeResetOffset(AtEnd())，必须在测试数据产生之前就已经订阅好。
set -euo pipefail

REPO_ROOT="${REPO_ROOT:-/mnt/d/repo/chat-server}"
NS=chat-system
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

log()  { echo -e "\n\033[1;36m==> $*\033[0m"; }
fail() { echo -e "\033[1;31m[FATAL] $*\033[0m" >&2; exit 1; }

wait_deploy() {
  local ns=$1 name=$2 timeout=${3:-180}
  kubectl -n "$ns" wait --for=condition=Available "deployment/$name" --timeout="${timeout}s" >/dev/null \
    || fail "等待 deployment/$name 就绪超时"
  echo "    deployment/$name 就绪"
}

wait_job() {
  local ns=$1 name=$2 timeout=${3:-180}
  kubectl -n "$ns" wait --for=condition=complete "job/$name" --timeout="${timeout}s" >/dev/null \
    || { kubectl -n "$ns" logs "job/$name" --tail=50 || true; fail "等待 job/$name 完成超时"; }
  echo "    job/$name 完成"
}

cd "$REPO_ROOT"

log "1/8 命名空间与配置"
kubectl apply -f deploy/k8s/namespace.yaml
kubectl -n "$NS" apply -f deploy/k8s/config/chat-configmap.yaml
kubectl -n "$NS" apply -f deploy/k8s/config/chat-secret.yaml
kubectl -n "$NS" apply -f deploy/k8s/config/mysql-initdb-configmap.yaml

log "2/8 中间件：MySQL / Redis / Kafka / etcd"
kubectl -n "$NS" apply -f deploy/k8s/middleware/mysql-statefulset.yaml
kubectl -n "$NS" apply -f deploy/k8s/middleware/redis-statefulset.yaml
kubectl -n "$NS" apply -f deploy/k8s/middleware/kafka-statefulset.yaml
kubectl -n "$NS" apply -f deploy/k8s/middleware/etcd-statefulset.yaml

kubectl -n "$NS" wait --for=condition=Ready pod -l app=mysql --timeout=240s >/dev/null || fail "MySQL 就绪超时"
kubectl -n "$NS" wait --for=condition=Ready pod -l app=redis --timeout=120s >/dev/null || fail "Redis 就绪超时"
kubectl -n "$NS" wait --for=condition=Ready pod -l app=kafka --timeout=240s >/dev/null || fail "Kafka 就绪超时"
kubectl -n "$NS" wait --for=condition=Ready pod -l app=etcd --timeout=120s >/dev/null || fail "etcd 就绪超时"
echo "    中间件全部就绪"

log "3/8 校验 MySQL 初始化（5 个拆分库 + svc_cdc 账号）"
MYSQL_POD=$(kubectl -n "$NS" get pod -l app=mysql -o jsonpath='{.items[0].metadata.name}')
# MySQL 首次启动要跑 --initialize-insecure + initdb 脚本，
# 期间 Pod 可能因为 CNI 重建 sandbox 而重启一次，这里做重试而不是一次性判定。
DB_OK=0
for i in $(seq 1 60); do
  MYSQL_POD=$(kubectl -n "$NS" get pod -l app=mysql -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
  if [ -n "$MYSQL_POD" ] && \
     kubectl -n "$NS" exec "$MYSQL_POD" -- mysql -uroot -proot123456 -Nse \
       "SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME IN ('chat_user','chat_apply','chat_friendship','chat_group','chat_message')" 2>/dev/null | tr -d ' \r' | grep -q '^5$'; then
    DB_OK=1
    break
  fi
  sleep 3
done
[ "$DB_OK" = "1" ] || {
  kubectl -n "$NS" logs "$MYSQL_POD" --tail=60 || true
  fail "5 个拆分库未全部建成（initdb 未执行？）"
}
for db in chat_user chat_apply chat_friendship chat_group chat_message; do
  echo "    库 $db 存在"
done
kubectl -n "$NS" exec "$MYSQL_POD" -- mysql -uroot -proot123456 -Nse \
  "SHOW GRANTS FOR 'svc_cdc'@'%'" 2>/dev/null | grep -q "REPLICATION SLAVE" \
  || fail "svc_cdc 缺少 REPLICATION SLAVE 权限"
echo "    svc_cdc 权限正确"

log "3b/8 校验 MySQL binlog/GTID（CDC 的前置条件）"
GTID=$(kubectl -n "$NS" exec "$MYSQL_POD" -- mysql -uroot -proot123456 -Nse \
  "SELECT @@global.gtid_mode, @@global.binlog_format, @@global.log_bin" 2>/dev/null | tr -d ' \r')
echo "    gtid_mode/binlog_format/log_bin = $GTID"
echo "$GTID" | grep -q "^ON" || fail "gtid_mode 未开启，CDC 无法启动"
echo "$GTID" | grep -q "ROW" || fail "binlog_format 不是 ROW，CDC 无法订阅 outbox"
echo "$GTID" | grep -q "1" || fail "log_bin 未开启"

log "4/8 Toxiproxy + 代理路由"
# 注意顺序：Job 的 spec 不可变，重新部署时必须先删掉旧的 Job 再 apply，
# 否则 apply 会因为 spec 相同而复用已完成的旧 Job（不会重新执行代理初始化）。
kubectl -n "$NS" delete job toxiproxy-init --ignore-not-found --wait=true >/dev/null
kubectl -n "$NS" apply -f deploy/k8s/middleware/toxiproxy.yaml
wait_deploy "$NS" toxiproxy 120
wait_job "$NS" toxiproxy-init 180

log "5/8 Envoy 配置（从 configs/ 生成，避免两份真相）"
kubectl -n "$NS" create configmap envoy-config \
  --from-file=envoy.yaml=configs/envoy.yaml \
  --from-file=proto.pb=configs/proto.pb \
  --dry-run=client -o yaml | kubectl apply -f -

log "6/8 业务服务"
kubectl -n "$NS" apply -f deploy/k8s/microservices/static-pvc.yaml
kubectl -n "$NS" apply -f deploy/k8s/microservices/user-service-deployment.yaml
kubectl -n "$NS" apply -f deploy/k8s/microservices/apply-service-deployment.yaml
kubectl -n "$NS" apply -f deploy/k8s/microservices/friendship-service-deployment.yaml
kubectl -n "$NS" apply -f deploy/k8s/microservices/group-service-deployment.yaml
kubectl -n "$NS" apply -f deploy/k8s/microservices/message-service-deployment.yaml
kubectl -n "$NS" apply -f deploy/k8s/microservices/chat-server-deployment.yaml
kubectl -n "$NS" apply -f deploy/k8s/microservices/envoy-deployment.yaml

for d in user-service apply-service friendship-service group-service message-service chat-server envoy; do
  wait_deploy "$NS" "$d" 180
done

# 镜像 tag 不变时 kubectl apply 不会触发滚动更新，
# 重新构建镜像后必须显式重启，否则跑的还是旧二进制。
log "6b/8 重启业务服务以加载最新镜像"
for d in user-service apply-service friendship-service group-service message-service chat-server; do
  kubectl -n "$NS" rollout restart "deployment/$d" >/dev/null
  echo "    restart deployment/$d"
done
for d in user-service apply-service friendship-service group-service message-service chat-server envoy; do
  wait_deploy "$NS" "$d" 180
done

log "7/8 CDC（必须在业务服务之后：否则 canal 订阅不到尚未建表的 outbox）"
kubectl -n "$NS" apply -f deploy/k8s/microservices/cdc-service-deployment.yaml
wait_deploy "$NS" cdc-service 180
sleep 5
kubectl -n "$NS" logs deployment/cdc-service --tail=30 | sed 's/^/    /'

log "8/8 集群概览"
kubectl -n "$NS" get pods -o wide
kubectl -n "$NS" get svc

echo -e "\n\033[1;32mDEPLOY_OK\033[0m"
