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

# 下载并安装 Xray-core
RUN mkdir -p /usr/local/xray && \
    curl -L https://github.com/XTLS/Xray-core/releases/download/v1.8.24/Xray-linux-64.zip -o /tmp/xray.zip && \
    unzip /tmp/xray.zip -d /usr/local/xray/ && \
    chmod +x /usr/local/xray/xray && \
    rm /tmp/xray.zip

# 创建必要的目录
RUN mkdir -p /etc/openvpn /etc/xray /var/log/xray

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# 暴露 Xray 默认监听端口
EXPOSE 10000

ENTRYPOINT ["/entrypoint.sh"]
