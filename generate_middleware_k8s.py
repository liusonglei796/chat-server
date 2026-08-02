import os

os.makedirs('deploy/k8s/middleware', exist_ok=True)

# 1. MySQL
with open('deploy/k8s/middleware/mysql-statefulset.yaml', 'w') as f:
    f.write("""apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mysql
  namespace: chat-system
spec:
  serviceName: "mysql"
  replicas: 1
  selector:
    matchLabels:
      app: mysql
  template:
    metadata:
      labels:
        app: mysql
    spec:
      containers:
        - name: mysql
          image: mysql:8.0
          ports:
            - containerPort: 3306
          env:
            - name: MYSQL_ROOT_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: chat-secrets
                  key: MYSQL_PASSWORD
            - name: MYSQL_DATABASE
              value: "chat"
          command: ["mysqld"]
          args:
            - "--default-authentication-plugin=mysql_native_password"
            - "--slow_query_log=ON"
            - "--long_query_time=1"
            - "--slow_query_log_file=/var/lib/mysql/slow-query.log"
          volumeMounts:
            - name: mysql-data
              mountPath: /var/lib/mysql
  volumeClaimTemplates:
    - metadata:
        name: mysql-data
      spec:
        accessModes: [ "ReadWriteOnce" ]
        resources:
          requests:
            storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: mysql
  namespace: chat-system
spec:
  ports:
    - port: 3306
  selector:
    app: mysql
""")

# 2. Redis
with open('deploy/k8s/middleware/redis-statefulset.yaml', 'w') as f:
    f.write("""apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis
  namespace: chat-system
spec:
  serviceName: "redis"
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
        - name: redis
          image: redis:7-alpine
          ports:
            - containerPort: 6379
          volumeMounts:
            - name: redis-data
              mountPath: /data
  volumeClaimTemplates:
    - metadata:
        name: redis-data
      spec:
        accessModes: [ "ReadWriteOnce" ]
        resources:
          requests:
            storage: 2Gi
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: chat-system
spec:
  ports:
    - port: 6379
  selector:
    app: redis
""")

# 3. Kafka (KRaft mode)
with open('deploy/k8s/middleware/kafka-statefulset.yaml', 'w') as f:
    f.write("""apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: kafka
  namespace: chat-system
spec:
  serviceName: "kafka"
  replicas: 1
  selector:
    matchLabels:
      app: kafka
  template:
    metadata:
      labels:
        app: kafka
    spec:
      containers:
        - name: kafka
          image: apache/kafka:4.1.1
          ports:
            - containerPort: 9092
            - containerPort: 9093
          env:
            - name: KAFKA_NODE_ID
              value: "1"
            - name: KAFKA_PROCESS_ROLES
              value: "broker,controller"
            - name: KAFKA_LISTENERS
              value: "PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093"
            - name: KAFKA_ADVERTISED_LISTENERS
              value: "PLAINTEXT://kafka.chat-system.svc.cluster.local:9092"
            - name: KAFKA_LISTENER_SECURITY_PROTOCOL_MAP
              value: "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT"
            - name: KAFKA_INTER_BROKER_LISTENER_NAME
              value: "PLAINTEXT"
            - name: KAFKA_CONTROLLER_LISTENER_NAMES
              value: "CONTROLLER"
            - name: KAFKA_CONTROLLER_QUORUM_VOTERS
              value: "1@kafka:9093"
            - name: KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR
              value: "1"
            - name: KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR
              value: "1"
            - name: KAFKA_TRANSACTION_STATE_LOG_MIN_ISR
              value: "1"
            - name: KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS
              value: "0"
            - name: KAFKA_NUM_PARTITIONS
              value: "3"
          volumeMounts:
            - name: kafka-data
              mountPath: /tmp/kafka-logs
  volumeClaimTemplates:
    - metadata:
        name: kafka-data
      spec:
        accessModes: [ "ReadWriteOnce" ]
        resources:
          requests:
            storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: kafka
  namespace: chat-system
spec:
  ports:
    - name: client
      port: 9092
    - name: controller
      port: 9093
  selector:
    app: kafka
""")

# 4. Etcd
with open('deploy/k8s/middleware/etcd-statefulset.yaml', 'w') as f:
    f.write("""apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: etcd
  namespace: chat-system
spec:
  serviceName: "etcd"
  replicas: 1
  selector:
    matchLabels:
      app: etcd
  template:
    metadata:
      labels:
        app: etcd
    spec:
      containers:
        - name: etcd
          image: bitnami/etcd:3.5
          ports:
            - containerPort: 2379
            - containerPort: 2380
          env:
            - name: ALLOW_NONE_AUTHENTICATION
              value: "yes"
            - name: ETCD_ADVERTISE_CLIENT_URLS
              value: "http://0.0.0.0:2379"
          volumeMounts:
            - name: etcd-data
              mountPath: /bitnami/etcd/data
  volumeClaimTemplates:
    - metadata:
        name: etcd-data
      spec:
        accessModes: [ "ReadWriteOnce" ]
        resources:
          requests:
            storage: 2Gi
---
apiVersion: v1
kind: Service
metadata:
  name: etcd
  namespace: chat-system
spec:
  ports:
    - name: client
      port: 2379
    - name: peer
      port: 2380
  selector:
    app: etcd
""")
