#!/usr/bin/env bash
# 诊断集群内 DNS 解析
set -uo pipefail
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
NS=chat-system

echo "==> chat-server 的 resolv.conf"
kubectl -n "$NS" exec deployment/chat-server -- cat /etc/resolv.conf 2>&1

echo
echo "==> 完整 nslookup friendship-service"
kubectl -n "$NS" exec deployment/chat-server -- nslookup friendship-service 2>&1 | tail -12

echo
echo "==> nslookup mysql（对照，能解析说明 DNS 本身没问题）"
kubectl -n "$NS" exec deployment/chat-server -- nslookup mysql 2>&1 | tail -8

echo
echo "==> CoreDNS 日志"
kubectl -n kube-system logs deployment/coredns --tail=20 2>&1 | tail -20

echo
echo "==> CoreDNS 就绪状态"
kubectl -n kube-system get pods -l k8s-app=kube-dns -o wide 2>&1
