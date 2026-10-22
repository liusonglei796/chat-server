#!/usr/bin/env bash
# 链路冒烟：注册 → 登录 → 申请 → 审批 → 拉黑 → 查库
# Pod 内只做 HTTP 调用，DB 查询由宿主脚本用 kubectl exec 完成（alpine 里没有 kubectl）。
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

kubectl -n "$NS" delete pod smoke --ignore-not-found >/dev/null 2>&1 || true

cat > /tmp/smoke.yaml <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: smoke
  namespace: chat-system
spec:
  restartPolicy: Never
  containers:
    - name: smoke
      image: alpine:3.18
      command: ["/bin/sh", "-c"]
      args:
        - |
          set -e
          GW=http://chat-server:8000
          EN=http://envoy:9080
          TAG=$(date +%s)
          U1="sm_a_$TAG"; U2="sm_b_$TAG"; PWD='Password123!'

          echo "== 1/2 注册 A/B"
          wget -q -O- --header='Content-Type: application/json' \
            --post-data="{\"username\":\"$U1\",\"password\":\"$PWD\",\"client_token\":\"c1\"}" "$GW/auth/register" || echo "  (注册 A 失败)"
          wget -q -O- --header='Content-Type: application/json' \
            --post-data="{\"username\":\"$U2\",\"password\":\"$PWD\",\"client_token\":\"c2\"}" "$GW/auth/register" || echo "  (注册 B 失败)"

          echo "== 3/4 登录"
          R1=$(wget -q -O- --header='Content-Type: application/json' \
            --post-data="{\"username\":\"$U1\",\"password\":\"$PWD\"}" "$GW/auth/login")
          ID1=$(echo "$R1" | sed -n 's/.*"uuid":"\([^"]*\)".*/\1/p')
          TOK1=$(echo "$R1" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
          R2=$(wget -q -O- --header='Content-Type: application/json' \
            --post-data="{\"username\":\"$U2\",\"password\":\"$PWD\"}" "$GW/auth/login")
          ID2=$(echo "$R2" | sed -n 's/.*"uuid":"\([^"]*\)".*/\1/p')
          TOK2=$(echo "$R2" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
          echo "  A=$ID1 B=$ID2"
          echo "SMOKE_IDS id1=$ID1 id2=$ID2"

          echo "== 5 A 申请加 B"
          wget -q -O- --header='Content-Type: application/json' --header="Authorization: Bearer $TOK1" \
            --post-data="{\"friend_id\":\"$ID2\",\"message\":\"hi\",\"client_token\":\"ap$TAG\"}" "$GW/friends/apply"; echo

          echo "== 6 B 审批"
          wget -q -O- --header='Content-Type: application/json' --header="Authorization: Bearer $TOK2" \
            --post-data="{\"applicant_id\":\"$ID1\"}" "$GW/friends/applies/approve"; echo

          echo "== 8 Envoy 路径拉黑"
          wget -q -O- --header='Content-Type: application/json' --header="x-user-id: $ID1" \
            --post-data="{\"user_id\":\"$ID1\",\"friend_id\":\"$ID2\"}" "$EN/friends/block"; echo
          echo "== SMOKE_HTTP_DONE"
EOF

kubectl apply -f /tmp/smoke.yaml >/dev/null
kubectl -n "$NS" wait --for=condition=Ready pod/smoke --timeout=120s >/dev/null 2>&1 || true
sleep 6
LOG=$(kubectl -n "$NS" logs smoke 2>&1)
echo "$LOG"

ID1=$(echo "$LOG" | sed -n 's/.*SMOKE_IDS id1=\([^ ]*\) id2=.*/\1/p')
ID2=$(echo "$LOG" | sed -n 's/.*SMOKE_IDS id1=[^ ]* id2=\(.*\)/\1/p')
echo "== 7 等待好友关系落库 (A=$ID1 B=$ID2)"
for i in $(seq 1 30); do
  N=$(kubectl -n "$NS" exec mysql-0 -- mysql -uroot -proot123456 -Nse \
    "SELECT COUNT(*) FROM chat_friendship.user_friendship WHERE (user_id='$ID1' AND friend_id='$ID2') OR (user_id='$ID2' AND friend_id='$ID1')" 2>/dev/null | tr -d ' \r')
  echo "   轮询 $i: rows=$N"
  [ "$N" = "2" ] && break
  sleep 2
done

echo "== 9 好友关系状态"
kubectl -n "$NS" exec mysql-0 -- mysql -uroot -proot123456 -e \
  "SELECT user_id, friend_id, status, version FROM chat_friendship.user_friendship WHERE user_id IN ('$ID1','$ID2')" 2>/dev/null
echo "== 10 outbox 事件"
kubectl -n "$NS" exec mysql-0 -- mysql -uroot -proot123456 -e \
  "SELECT event_type, payload FROM chat_friendship.outbox ORDER BY created_at DESC LIMIT 5" 2>/dev/null

kubectl -n "$NS" delete pod smoke --ignore-not-found >/dev/null 2>&1 || true
