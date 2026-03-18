# 多阶段构建：编译 → 运行
FROM golang:1.21-alpine AS builder

WORKDIR /app

# 缓存依赖层
COPY go.mod go.sum ./
RUN go mod download

# 编译应用
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

# 运行阶段 - 使用精简镜像
FROM alpine:3.18

RUN apk add --no-cache ca-certificates curl

WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/app .

# 非 root 用户运行
RUN addgroup -S appuser && adduser -S appuser -G appuser
USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["./app"]
