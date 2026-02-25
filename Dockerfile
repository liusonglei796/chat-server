# 设置 Docker 镜像代理 + Go 模块代理
# 阶段 1: 构建阶段
FROM golang:1.24-alpine AS builder

# 设置 Go 模块代理（使用国内镜像）
ENV GOPROXY=https://goproxy.cn,direct

# 安装构建所需工具
RUN apk add --no-cache gcc musl-dev git

# 设置工作目录
WORKDIR /app

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建可执行文件
RUN CGO_ENABLED=1 GOOS=linux go build -o kama_chat_server ./cmd/kama_chat_server/main.go

# 阶段 2: 运行阶段
FROM alpine:latest

# 安装运行时依赖 (ca-certificates 用于 HTTPS, tzdata 用于时区)
RUN apk add --no-cache ca-certificates tzdata

# 设置工作目录
WORKDIR /app

# 从构建阶段复制可执行文件
COPY --from=builder /app/kama_chat_server .

# 复制配置文件
COPY configs/config.toml .

# 创建日志目录
RUN mkdir -p /app/logs

# 暴露端口
EXPOSE 8000

# 启动命令
ENTRYPOINT ["./kama_chat_server"]
