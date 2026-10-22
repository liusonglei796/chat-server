#!/usr/bin/env bash
# 找出是谁触发了 WSL 的关机
set -uo pipefail

echo "==> journal 中与关机/重启相关的记录"
journalctl --no-pager -o short-iso 2>/dev/null \
  | grep -i -E "poweroff|powering off|shutdown.target|Reached target|Requested transaction|Stopping |Reboot" \
  | tail -25

echo
echo "==> PID1 与 systemd 会话"
ps -p 1 -o pid,comm,args --no-headers
loginctl list-sessions --no-legend 2>/dev/null || true

echo
echo "==> WSL 配置"
cat /etc/wsl.conf 2>/dev/null || echo "(无 /etc/wsl.conf)"
echo "--- /mnt/c/Users/*/.wslconfig ---"
ls -la /mnt/c/Users/*/.wslconfig 2>/dev/null || echo "(无 .wslconfig)"
for f in /mnt/c/Users/*/.wslconfig; do
  [ -f "$f" ] && { echo "### $f"; cat "$f"; }
done

echo
echo "==> WSL 版本"
cat /proc/sys/kernel/osrelease 2>/dev/null || true
