#!/bin/sh
echo "--- LOGIN TEST ---"
RESPONSE=$(wget -qO- --post-data='{"telephone":"13800138000","password":"password"}' --header='Content-Type: application/json' http://localhost:8000/auth/login)
echo "Login response: $RESPONSE"

TOKEN=$(echo "$RESPONSE" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)
if [ -n "$TOKEN" ]; then
  echo "--- LOGOUT TEST ---"
  wget -qO- --post-data='' --header="Authorization: Bearer $TOKEN" http://localhost:8000/auth/logout
  echo ""
  echo "--- KICK TEST ---"
  wget -qO- --post-data='{"user_id":"test1234567890"}' --header="Authorization: Bearer $TOKEN" --header='Content-Type: application/json' http://localhost:8000/auth/kick
  echo ""
else
  echo "No token obtained"
fi
