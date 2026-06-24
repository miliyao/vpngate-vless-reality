# ── 阶段 1：前端编译构建 ────────────────────────────────────────────────────────
FROM node:18.20-slim AS frontend-builder

WORKDIR /app/frontend

# 拷贝前端 package 配置并安装编译依赖
COPY web/frontend/package*.json ./
RUN npm install

# 拷贝前端源码并构建静态资源包
COPY web/frontend/ ./
RUN NODE_OPTIONS="--max-old-space-size=512" npm run build

# ── 阶段 2：Go 后端编译构建 ──────────────────────────────────────────────────────
FROM golang:alpine AS backend-builder

WORKDIR /app/backend-go

# 拷贝依赖配置并下载 Go 模块包
COPY web/backend-go/go.mod web/backend-go/go.sum ./
RUN go mod download

# 拷贝后端 Go 核心源码并编译成无 CGO 依赖的静态二进制
COPY web/backend-go/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o vless-panel main.go

# ── 阶段 3：轻量级运行时镜像 ──────────────────────────────────────────────────────
FROM alpine:latest AS runtime

# 安装基础依赖包 (curl) 用于容器健康度检查，配置本地时区
RUN apk add --no-cache curl tzdata

WORKDIR /app

# 拷贝 Go 二进制主程序
COPY --from=backend-builder /app/backend-go/vless-panel ./vless-panel

# 拷贝打包好的前端静态资源文件 dist，Go 主程序将自适应探测并托管该文件夹
COPY --from=frontend-builder /app/backend/dist ./dist

# 声明挂载卷
VOLUME ["/app/data", "/app/config"]

EXPOSE 3000

# 运行 Go 高性能一体化服务端
CMD ["./vless-panel"]
