#!/bin/bash

# VLESS Reality 多出口代理系统 VPS 一键部署脚本
# 中文输出日志，保证国内开发者的运维友好度

set -e

# 配色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PLAIN='\033[0m'

echo -e "${BLUE}====================================================${PLAIN}"
echo -e "${GREEN}    VLESS Reality + VPNGate 多出口自愈面板一键部署脚本${PLAIN}"
echo -e "${BLUE}====================================================${PLAIN}"

# 1. 权限检查
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}[!] 错误：请使用 root 权限或 sudo 运行此脚本！${PLAIN}"
    exit 1
fi

# 2. 系统包管理器判断与基础依赖安装
echo -e "${YELLOW}[*] 正在检查并安装基础依赖 (curl, git, iptables)...${PLAIN}"
if [ -f /etc/debian_version ]; then
    apt-get update -y
    apt-get install -y curl git iptables unzip
elif [ -f /etc/redhat-release ]; then
    yum install -y curl git iptables unzip
else
    echo -e "${YELLOW}[!] 警告：未知的 Linux 发行版，跳过系统依赖包自动安装${PLAIN}"
fi

# 2.5 自动克隆项目仓库（支持直接 curl 一键拉起）
if [ ! -d "./docker/egress-image" ] || [ ! -f "docker-compose.yml" ]; then
    echo -e "${YELLOW}[*] 检测到当前目录缺少项目文件，正在为您拉取最新 GitHub 仓库...${PLAIN}"
    rm -rf vpngate-vless-reality
    if ! git clone https://github.com/miliyao/vpngate-vless-reality.git; then
        echo -e "${RED}[!] 错误：无法克隆 GitHub 仓库，请检查服务器网络或 GitHub 连通性！${PLAIN}"
        exit 1
    fi
    cd vpngate-vless-reality
    chmod +x deploy.sh
    exec ./deploy.sh
fi

# 3. 检查 Docker 环境
echo -e "${YELLOW}[*] 正在检查 Docker 安装状态...${PLAIN}"
if ! command -v docker &> /dev/null; then
    echo -e "${YELLOW}[*] 未检测到 Docker，正在执行 Docker 官方一键安装脚本...${PLAIN}"
    curl -fsSL https://get.docker.com | bash
    systemctl start docker
    systemctl enable docker
    echo -e "${GREEN}[+] Docker 安装并启动成功！${PLAIN}"
else
    echo -e "${GREEN}[+] 检测到 Docker 已安装${PLAIN}"
fi

# 4. 检查 Docker Compose 插件
echo -e "${YELLOW}[*] 正在检查 Docker Compose 支持...${PLAIN}"
COMPOSE_CMD=""
if docker compose version &> /dev/null; then
    COMPOSE_CMD="docker compose"
    echo -e "${GREEN}[+] 检测到 Docker Compose v2 插件支持${PLAIN}"
elif command -v docker-compose &> /dev/null; then
    COMPOSE_CMD="docker-compose"
    echo -e "${GREEN}[+] 检测到旧版 docker-compose 工具支持${PLAIN}"
else
    echo -e "${YELLOW}[*] 未检测到 Docker Compose，正在自动下载 Compose 插件...${PLAIN}"
    mkdir -p /usr/local/lib/docker/cli-plugins/
    curl -SL "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/lib/docker/cli-plugins/docker-compose
    chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
    COMPOSE_CMD="docker compose"
    echo -e "${GREEN}[+] Docker Compose 插件下载并配置成功！${PLAIN}"
fi

# 5. 构建基础出口镜像 vpngate-egress:latest
echo -e "${YELLOW}[*] 正在构建底层出口 VPN 桥接镜像 (vpngate-egress:latest)...${PLAIN}"
if [ ! -d "./docker/egress-image" ]; then
    echo -e "${RED}[!] 错误：当前目录下未找到 ./docker/egress-image，请确保在项目根目录下运行此脚本！${PLAIN}"
    exit 1
fi

docker build -t vpngate-egress:latest ./docker/egress-image/
echo -e "${GREEN}[+] 出口镜像构建成功！${PLAIN}"

# 6. 初始化持久化目录结构
echo -e "${YELLOW}[*] 正在初始化数据挂载目录...${PLAIN}"
mkdir -p ./data/egress
mkdir -p ./config

# 7. 一键拉起 Docker Compose 编排
echo -e "${YELLOW}[*] 正在启动控制面板容器组...${PLAIN}"
$COMPOSE_CMD down &> /dev/null || true
$COMPOSE_CMD up -d

# 8. 获取 VPS 外网 IP
echo -e "${YELLOW}[*] 正在获取 VPS 公网 IP 地址...${PLAIN}"
VPS_IP=$(curl -s --max-time 5 https://ipinfo.io/ip || curl -s --max-time 5 https://api.ipify.org || echo "你的服务器IP")

echo -e "${BLUE}====================================================${PLAIN}"
echo -e "${GREEN}[+] 部署全部就绪！${PLAIN}"
echo -e "${BLUE}====================================================${PLAIN}"
echo -e "控制面板访问地址: ${GREEN}http://${VPS_IP}:${PANEL_PORT:-3000}${PLAIN}"
echo -e "默认 Reality 混淆域名: ${YELLOW}www.asus.com${PLAIN}"
echo -e ""
echo -e "操作指引："
echo -e "  1. 浏览器打开上面的面板地址。"
echo -e "  2. 输入出口名称 (例如 jp-01)，选择目标国家/地区，点击一键构建。"
echo -e "  3. 当出口卡片状态变为 ${GREEN}运行中${PLAIN} 且获取到动态出口 IP 后，即可复制 VLESS 订阅链接连接使用。"
echo -e "  4. 若需修改混淆域名或端口，可随时编辑根目录下的 ${YELLOW}docker-compose.yml${PLAIN} 并重启面板。"
echo -e "  5. 容器后台定时器每 60 秒会自动对出口连通性进行自愈检测，失效节点将自动“透明漂移”无需人工维护。"
echo -e "${BLUE}====================================================${PLAIN}"
