# 多阶段构建：编译 → 运行
FROM golang:1.25-alpine AS builder

WORKDIR /app

ENV GOPROXY=https://goproxy.cn,direct

# 缓存依赖层
COPY go.mod go.sum ./
RUN go mod download

# 编译应用
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app ./cmd/chat_server

# 运行阶段 - 使用精简镜像
FROM alpine:3.18

WORKDIR /app

# 创建日志和静态目录
RUN mkdir -p /app/logs /app/static/avatars /app/static/files

# 复制静态文件（前端）
COPY --from=builder /app/web /app/web

# 从构建阶段复制二进制文件
COPY --from=builder /app/app .

# 非 root 用户运行
RUN addgroup -S appuser && adduser -S appuser -G appuser && \
    chown -R appuser:appuser /app/logs /app/static
USER appuser

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8000/metrics || exit 1

ENTRYPOINT ["./app"]
