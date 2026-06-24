# ── 阶段 1：前端编译构建 ────────────────────────────────────────────────────────
FROM node:18.20-slim AS frontend-builder

WORKDIR /app/frontend

# 拷贝前端 package 配置并安装完整编译依赖
COPY web/frontend/package*.json ./
RUN npm install

# 拷贝前端源码并进行静态包构建
COPY web/frontend/ ./
RUN NODE_OPTIONS="--max-old-space-size=512" npm run build

# ── 阶段 2：后端与服务运行 ────────────────────────────────────────────────────────
FROM node:18.20-slim AS runtime

# 安装 openssl，主要用于 Reality 密钥对生成或一些基础网络诊断
RUN apt-get update && apt-get install -y --no-install-recommends \
    openssl \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 拷贝后端依赖声明并安装生产依赖
COPY web/backend/package*.json ./
RUN npm install --omit=dev

# 拷贝后端核心源码
COPY web/backend/ ./

# 将阶段 1 编译的前端静态产物拷贝至后端静态目录下，脱离 Git 编译维护
COPY --from=frontend-builder /app/backend/dist ./dist

# 暴露面板管理端口
EXPOSE 3000

CMD ["npm", "start"]
