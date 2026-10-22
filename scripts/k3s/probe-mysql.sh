#!/usr/bin/env bash
# 本地复现 MySQL 启动参数错误
set -uo pipefail
cd /tmp
rm -rf mysqltest && mkdir -p mysqltest
k3s-nerdctl run --rm \
  --name mysqltest \
  -e MYSQL_ROOT_PASSWORD=root123456 \
  -e MYSQL_DATABASE=chat \
  -v /tmp/mysqltest:/var/lib/mysql \
  mysql:8.0 \
  mysqld \
  --default-authentication-plugin=mysql_native_password \
  --server-id=1 \
  --log-bin=mysql-bin \
  --binlog-format=ROW \
  --binlog-row-image=FULL \
  --gtid-mode=ON \
  --enforce-gtid-consistency=ON \
  --slow_query_log=ON \
  --long_query_time=1 \
  --slow_query_log_file=/var/lib/mysql/slow-query.log
echo "EXIT=$?"
