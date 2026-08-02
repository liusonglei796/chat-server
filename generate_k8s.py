import os

os.makedirs('deploy/k8s/config', exist_ok=True)
os.makedirs('deploy/k8s/microservices', exist_ok=True)
os.makedirs('deploy/k8s/ingress', exist_ok=True)

# 1. Namespace
with open('deploy/k8s/namespace.yaml', 'w') as f:
    f.write("""apiVersion: v1
kind: Namespace
metadata:
  name: chat-system
""")

# 2. ConfigMap
with open('deploy/k8s/config/chat-configmap.yaml', 'w') as f:
    f.write("""apiVersion: v1
kind: ConfigMap
metadata:
  name: chat-config
  namespace: chat-system
data:
  config.toml: |
    [mainConfig]
    appName = "chat-server"
    host = "0.0.0.0"
    port = 8000

    [mysqlConfig]
    host = "mysql.default.svc.cluster.local"
    port = 3306
    user = "root"
    password = "${MYSQL_PASSWORD}"
    databaseName = "chat"

    [redisConfig]
    host = "redis.default.svc.cluster.local"
    port = 6379
    password = "${REDIS_PASSWORD}"
    db = 0

    [logConfig]
    logPath = "/app/logs"

    [kafkaConfig]
    hostPort = "kafka.default.svc.cluster.local:9092"
    loginTopic = "login"
    chatTopic = "chat_message"
    logoutTopic = "logout"
    partition = 0
    timeout = 1

    [staticSrcConfig]
    staticAvatarPath = "/app/static/avatars"
    staticFilePath = "/app/static/files"

    [jwtConfig]
    secret = "${JWT_SECRET}"
    accessTokenExpiry = 15
    refreshTokenExpiry = 168

    [snowflakeConfig]
    machineId = 1

    [otelConfig]
    endpoint = "otel-collector.default.svc.cluster.local:4317"
    serviceName = "chat-server"
    enabled = false
""")

# 3. Secret
import base64
mysql_pw = base64.b64encode(b"root123456").decode('utf-8')
redis_pw = base64.b64encode(b"").decode('utf-8')
jwt_sec = base64.b64encode(b"chat-super-secret-key-change-in-production").decode('utf-8')

with open('deploy/k8s/config/chat-secret.yaml', 'w') as f:
    f.write(f"""apiVersion: v1
kind: Secret
metadata:
  name: chat-secrets
  namespace: chat-system
type: Opaque
data:
  MYSQL_PASSWORD: {mysql_pw}
  REDIS_PASSWORD: {redis_pw}
  JWT_SECRET: {jwt_sec}
""")

# 4. Microservices
services = [
    {"name": "user-service", "port": 50051},
    {"name": "auth-service", "port": 50052},
    {"name": "relation-service", "port": 50053},
    {"name": "message-service", "port": 50054},
]

gateway = {"name": "chat-server", "port": 8000}

def generate_deployment(name, port, is_gateway=False):
    volumes = """
      volumes:
        - name: config-volume
          configMap:
            name: chat-config
"""
    volume_mounts = """
            - name: config-volume
              mountPath: /app/configs/config.toml
              subPath: config.toml
"""
    if is_gateway:
        volumes += """        - name: static-volume
          persistentVolumeClaim:
            claimName: chat-static-pvc
"""
        volume_mounts += """            - name: static-volume
              mountPath: /app/static
"""

    return f"""apiVersion: apps/v1
kind: Deployment
metadata:
  name: {name}
  namespace: chat-system
  labels:
    app: {name}
spec:
  replicas: 2
  selector:
    matchLabels:
      app: {name}
  template:
    metadata:
      labels:
        app: {name}
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchExpressions:
                    - key: app
                      operator: In
                      values:
                        - {name}
                topologyKey: "kubernetes.io/hostname"
      containers:
        - name: {name}
          image: myregistry.com/{name}:latest
          imagePullPolicy: Always
          ports:
            - containerPort: {port}
          env:
            - name: ETCD_ENDPOINTS
              value: "etcd.default.svc.cluster.local:2379"
          resources:
            requests:
              cpu: "100m"
              memory: "128Mi"
            limits:
              cpu: "500m"
              memory: "512Mi"
          readinessProbe:
            tcpSocket:
              port: {port}
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            tcpSocket:
              port: {port}
            initialDelaySeconds: 15
            periodSeconds: 20
          volumeMounts:{volume_mounts}{volumes}
---
apiVersion: v1
kind: Service
metadata:
  name: {name}
  namespace: chat-system
spec:
  selector:
    app: {name}
  ports:
    - protocol: TCP
      port: {port}
      targetPort: {port}
"""

for svc in services:
    with open(f"deploy/k8s/microservices/{svc['name']}-deployment.yaml", 'w') as f:
        f.write(generate_deployment(svc['name'], svc['port']))

with open(f"deploy/k8s/microservices/chat-server-deployment.yaml", 'w') as f:
    f.write(generate_deployment(gateway['name'], gateway['port'], True))

with open(f"deploy/k8s/microservices/static-pvc.yaml", 'w') as f:
    f.write("""apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: chat-static-pvc
  namespace: chat-system
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 10Gi
""")

# 5. Ingress
with open('deploy/k8s/ingress/chat-ingress.yaml', 'w') as f:
    f.write("""apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: chat-ingress
  namespace: chat-system
  annotations:
    kubernetes.io/ingress.class: "nginx"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
spec:
  rules:
    - host: chat.yourdomain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: chat-server
                port:
                  number: 8000
""")

