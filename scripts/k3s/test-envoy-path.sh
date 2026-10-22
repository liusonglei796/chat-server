#!/usr/bin/env bash
# 用有效好友对验证 Envoy 拉黑路径，抓完整响应
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

TAG=$(date +%s)
U1="env_a_$TAG"; U2="env_b_$TAG"; PWD='Password123!'
GW=http://chat-server:8000
EN=http://envoy:9080

echo "==> 准备好友对（走 Gin 网关，避开 Envoy 的 apply 路由问题）"
curl -s -X POST "$GW/auth/register" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$U1\",\"password\":\"$PWD\",\"client_token\":\"c1\"}" >/dev/null
curl -s -X POST "$GW/auth/register" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$U2\",\"password\":\"$PWD\",\"client_token\":\"c2\"}" >/dev/null

R1=$(curl -s -X POST "$GW/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"$U1\",\"password\":\"$PWD\"}")
ID1=$(printf '%s' "$R1" | grep -o '"uuid":"[^"]*"' | head -1 | cut -d'"' -f4)
TOK1=$(printf '%s' "$R1" | grep -o '"access_token":"[^"]*"' | head -1 | cut -d'"' -f4)
R2=$(curl -s -X POST "$GW/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"$U2\",\"password\":\"$PWD\"}")
ID2=$(printf '%s' "$R2" | grep -o '"uuid":"[^"]*"' | head -1 | cut -d'"' -f4)
TOK2=$(printf '%s' "$R2" | grep -o '"access_token":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "  A=$ID1 B=$ID2"

curl -s -X POST "$GW/friends/apply" -H 'Content-Type: application/json' -H "Authorization: Bearer $TOK1" \
  -d "{\"friend_id\":\"$ID2\",\"message\":\"hi\",\"client_token\":\"ap$TAG\"}" >/dev/null
curl -s -X POST "$GW/friends/applies/approve" -H 'Content-Type: application/json' -H "Authorization: Bearer $TOK2" \
  -d "{\"applicant_id\":\"$ID1\"}" >/dev/null

for i in $(seq 1 20); do
  N=$(kubectl -n "$NS" exec mysql-0 -- mysql -uroot -proot123456 -Nse \
    "SELECT COUNT(*) FROM chat_friendship.user_friendship WHERE (user_id='$ID1' AND friend_id='$ID2') OR (user_id='$ID2' AND friend_id='$ID1')" 2>/dev/null | tr -d ' \r')
  [ "$N" = "2" ] && { echo "  好友关系就绪 (${i}s)"; break; }
  sleep 1
done

echo
echo "== A) Gin 网关拉黑（对照组）"
curl -s -i -X PUT "$GW/friends/block" -H 'Content-Type: application/json' -H "Authorization: Bearer $TOK1" \
  -d "{\"friend_id\":\"$ID2\"}" | sed -n '1p;/^{/p'

echo
echo "== B) Envoy 拉黑（PUT）"
curl -s -i -X PUT "$EN/friends/block" -H 'Content-Type: application/json' -H "x-user-id: $ID1" \
  -d "{\"user_id\":\"$ID1\",\"friend_id\":\"$ID2\"}" | sed -n '1,12p'

echo
echo "== C) 状态"
kubectl -n "$NS" exec mysql-0 -- mysql -uroot -proot123456 -e \
  "SELECT user_id, friend_id, status, version FROM chat_friendship.user_friendship WHERE user_id IN ('$ID1','$ID2')" 2>/dev/null

echo
echo "== D) outbox"
kubectl -n "$NS" exec mysql-0 -- mysql -uroot -proot123456 -e \
  "SELECT event_type, payload FROM chat_friendship.outbox ORDER BY created_at DESC LIMIT 3" 2>/dev/null
