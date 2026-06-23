FROM debian:12-slim

# 设置时区和环境变量
ENV TZ=Asia/Shanghai
ENV DEBIAN_FRONTEND=noninteractive

# 安装基础依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    openvpn \
    curl \
    ca-certificates \
    unzip \
    iptables \
    iproute2 \
    procps \
    && rm -rf /var/lib/apt/lists/*

# Xray-core 版本参数：默认动态拉取 GitHub latest release，也可在 build 时覆盖：
#   docker build --build-arg XRAY_VERSION=v26.3.27 .
ARG XRAY_VERSION=

# 下载并安装 Xray-core，按 VPS CPU 架构选择正确二进制
RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    case "$arch" in \
      amd64) xray_asset="Xray-linux-64.zip" ;; \
      arm64) xray_asset="Xray-linux-arm64-v8a.zip" ;; \
      armhf) xray_asset="Xray-linux-arm32-v7a.zip" ;; \
      *) echo "Unsupported architecture: $arch" >&2; exit 1 ;; \
    esac; \
    if [ -z "${XRAY_VERSION}" ]; then \
      XRAY_VERSION="$(curl -fsSL https://api.github.com/repos/XTLS/Xray-core/releases/latest \
        | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"; \
      echo "[+] 动态获取 Xray-core 最新版本: ${XRAY_VERSION}"; \
    fi; \
    mkdir -p /usr/local/xray && \
    curl -fL "https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/${xray_asset}" -o /tmp/xray.zip && \
    unzip /tmp/xray.zip -d /usr/local/xray/ && \
    chmod +x /usr/local/xray/xray && \
    rm /tmp/xray.zip && \
    echo "[+] Xray-core ${XRAY_VERSION} 安装完成" && \
    /usr/local/xray/xray version

# 创建必要的目录
RUN mkdir -p /etc/openvpn /etc/xray /var/log/xray

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# 暴露 Xray 默认监听端口
EXPOSE 10000

ENTRYPOINT ["/entrypoint.sh"]
