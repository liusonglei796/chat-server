#!/usr/bin/env bash
# 决定性实验：friendship-service Service 的 selector 在
#   (a) app=toxiproxy（劫持）  (b) app=friendship（真实 Pod）
# 之间切换，对比 Envoy 拉黑是否成功。
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

cat > /tmp/blocktest.yaml <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: blocktest
  namespace: chat-system
spec:
  restartPolicy: Never
  containers:
    - name: t
      image: alpine:3.18
      command: ["/bin/sh","-c"]
      args:
        - |
          set -e
          GW=http://chat-server:8000
          EN=http://envoy:9080
          TAG=$(date +%s)
          U1="x_a_$TAG"; U2="x_b_$TAG"; PWD='Password123!'
          post() { wget -q -O- --header='Content-Type: application/json' --post-data="$2" "$1"; }
          post "$GW/auth/register" "{\"username\":\"$U1\",\"password\":\"$PWD\",\"client_token\":\"c1\"}" >/dev/null || true
          post "$GW/auth/register" "{\"username\":\"$U2\",\"password\":\"$PWD\",\"client_token\":\"c2\"}" >/dev/null || true
          R1=$(post "$GW/auth/login" "{\"username\":\"$U1\",\"password\":\"$PWD\"}")
          ID1=$(echo "$R1" | grep -o '"uuid":"[^"]*"' | head -1 | cut -d'"' -f4)
          TOK1=$(echo "$R1" | grep -o '"access_token":"[^"]*"' | head -1 | cut -d'"' -f4)
          R2=$(post "$GW/auth/login" "{\"username\":\"$U2\",\"password\":\"$PWD\"}")
          ID2=$(echo "$R2" | grep -o '"uuid":"[^"]*"' | head -1 | cut -d'"' -f4)
          TOK2=$(echo "$R2" | grep -o '"access_token":"[^"]*"' | head -1 | cut -d'"' -f4)
          echo "IDS id1=$ID1 id2=$ID2"
          wget -q -O- --header='Content-Type: application/json' --header="Authorization: Bearer $TOK1" \
            --post-data="{\"friend_id\":\"$ID2\",\"message\":\"hi\",\"client_token\":\"ap$TAG\"}" "$GW/friends/apply" >/dev/null || true
          wget -q -O- --header='Content-Type: application/json' --header="Authorization: Bearer $TOK2" \
            --post-data="{\"applicant_id\":\"$ID1\"}" "$GW/friends/applies/approve" >/dev/null || true
          echo "APPROVED"
          # 等待异步建链（apply -> outbox -> CDC -> Kafka -> friendship_service）
          sleep 8
          BODY="{\"user_id\":\"$ID1\",\"friend_id\":\"$ID2\"}"
          LEN=$(printf '%s' "$BODY" | wc -c)
          echo "== Envoy PUT /friends/block"
          printf 'PUT /friends/block HTTP/1.1\r\nHost: envoy\r\nContent-Type: application/json\r\nx-user-id: %s\r\nContent-Length: %s\r\nConnection: close\r\n\r\n%s' \
            "$ID1" "$LEN" "$BODY" | nc envoy 9080
          echo "== ENVOY_BLOCK_DONE"
EOF

run_block() {
  local label="$1"
  kubectl -n "$NS" delete pod blocktest --ignore-not-found >/dev/null 2>&1 || true
  kubectl apply -f /tmp/blocktest.yaml >/dev/null
  local LOG=""
  for i in $(seq 1 40); do
    sleep 2
    LOG=$(kubectl -n "$NS" logs blocktest 2>&1 || true)
    echo "$LOG" | grep -q 'ENVOY_BLOCK_DONE' && break
  done
  echo "$LOG" | sed 's/^/    /'
  local IDS
  IDS=$(echo "$LOG" | grep -o 'IDS id1=[^ ]* id2=[^ ]*')
  local ID1 ID2
  ID1=$(echo "$IDS" | sed -n 's/.*id1=\([^ ]*\).*/\1/p')
  ID2=$(echo "$IDS" | sed -n 's/.*id2=//p')
  echo "  [$label] DB 状态:"
  kubectl -n "$NS" exec mysql-0 -- mysql -uroot -proot123456 -Nse \
    "SELECT user_id, status, version FROM chat_friendship.user_friendship WHERE user_id IN ('$ID1','$ID2')" 2>/dev/null | sed 's/^/    /'
  echo "  [$label] outbox: $(kubectl -n "$NS" exec mysql-0 -- mysql -uroot -proot123456 -Nse "SELECT event_type FROM chat_friendship.outbox ORDER BY created_at DESC LIMIT 1" 2>/dev/null | tr -d ' \r')"
  kubectl -n "$NS" delete pod blocktest --ignore-not-found >/dev/null 2>&1 || true
}

echo "############ A) selector = app=toxiproxy（劫持）"
kubectl -n "$NS" patch svc friendship-service --type=merge -p '{"spec":{"selector":{"app":"toxiproxy"}}}' >/dev/null
sleep 3
kubectl -n "$NS" get endpoints friendship-service 2>&1 | tail -1 | sed 's/^/  endpoints: /'
run_block "toxiproxy"

echo
echo "############ B) selector = app=friendship（真实 Pod）"
kubectl -n "$NS" patch svc friendship-service --type=merge -p '{"spec":{"selector":{"app":"friendship"}}}' >/dev/null
sleep 3
kubectl -n "$NS" get endpoints friendship-service 2>&1 | tail -1 | sed 's/^/  endpoints: /'
run_block "direct"
