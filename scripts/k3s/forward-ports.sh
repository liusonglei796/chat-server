#!/bin/bash
while true; do
  kubectl port-forward -n chat-system svc/toxiproxy 8474:8474 19080:19080 18000:18000 --address 0.0.0.0
  sleep 1
done
