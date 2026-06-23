#!/bin/bash

# VLESS Reality 多出口面板一键卸载脚本
# 支持 curl | bash 直接执行，也支持本地 bash scripts/uninstall.sh 执行
#
# 默认模式：停止容器、删除镜像和网络，保留 data/ 和 .env（可重新部署后还原数据）
# 彻底模式：追加 --purge 参数后，同时删除数据目录和项目目录
#
# 用法示例：
#   本地执行:   bash scripts/uninstall.sh [--purge]
#   curl 执行:  curl -fsSL https://raw.githubusercontent.com/miliyao/vpngate-vless-reality/main/scripts/uninstall.sh | bash -s -- [--purge]

set -euo pipefail

# ── 配色 ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
PLAIN='\033[0m'

# ── ROOT_DIR 推导（兼容 curl | bash 管道执行）───────────────────────────────
# curl | bash 模式下 BASH_SOURCE[0] 为空或为 /dev/stdin，无法用于路径推导。
# 改为按优先级自动探测项目根目录：
#   1. 环境变量 VLESS_ROOT 显式指定
#   2. 当前工作目录（用户 cd 到项目目录后执行）
#   3. ~/vpngate-vless-reality（deploy.sh 默认 clone 位置）
find_root_dir() {
  if [ -n "${VLESS_ROOT:-}" ] && [ -f "${VLESS_ROOT}/docker-compose.yml" ]; then
    echo "${VLESS_ROOT}"
    return
  fi
  if [ -f "$(pwd)/docker-compose.yml" ] && [ -d "$(pwd)/docker" ]; then
    echo "$(pwd)"
    return
  fi
  local default_clone="${HOME}/vpngate-vless-reality"
  if [ -f "${default_clone}/docker-compose.yml" ]; then
    echo "${default_clone}"
    return
  fi
  # 找不到，返回空
  echo ""
}

ROOT_DIR="$(find_root_dir)"

# ── 参数解析 ─────────────────────────────────────────────────────────────────
PURGE=false
for arg in "$@"; do
  case "$arg" in
    --purge) PURGE=true ;;
    --help|-h)
      echo "用法: bash scripts/uninstall.sh [--purge]"
      echo "      curl -fsSL <URL>/scripts/uninstall.sh | bash -s -- [--purge]"
      echo ""
      echo "  默认       停止并删除所有容器、镜像、Docker 网络，保留 data/ 和 .env"
      echo "  --purge    在默认操作基础上，同时删除 data/、.env 及整个项目目录"
      echo ""
      echo "  环境变量:"
      echo "  VLESS_ROOT=<path>  显式指定项目根目录（curl 执行时推荐使用）"
      exit 0
      ;;
  esac
done

# ── 开场提示 ──────────────────────────────────────────────────────────────────
echo -e "${BLUE}====================================================${PLAIN}"
echo -e "${RED}    VLESS Reality 面板卸载脚本${PLAIN}"
echo -e "${BLUE}====================================================${PLAIN}"

if [ -z "${ROOT_DIR}" ]; then
  echo -e "${RED}[!] 未能找到项目目录（docker-compose.yml 不存在）${PLAIN}"
  echo -e "${YELLOW}[*] 请在项目根目录下执行，或通过环境变量指定路径：${PLAIN}"
  echo -e "${YELLOW}    VLESS_ROOT=/path/to/vpngate-vless-reality bash <(curl -fsSL ...)${PLAIN}"
  # 即使找不到项目目录，也继续清理 Docker 资源（容器/镜像）
  echo -e "${YELLOW}[*] 将仅清理 Docker 容器和镜像，跳过目录相关操作${PLAIN}"
  SKIP_DIR_OPS=true
else
  SKIP_DIR_OPS=false
  echo -e "${GREEN}[*] 项目根目录: ${ROOT_DIR}${PLAIN}"
fi

if [ "${PURGE}" = true ]; then
  echo -e "${RED}[!] 彻底模式（--purge）：数据目录和项目目录将被永久删除！${PLAIN}"
else
  echo -e "${YELLOW}[*] 默认模式：将删除容器和镜像，保留 data/ 和 .env${PLAIN}"
  echo -e "${YELLOW}[*] 如需彻底清除，请追加 --purge 参数${PLAIN}"
fi

echo ""
# curl | bash 模式下 stdin 被管道占用，改为从 /dev/tty 读取用户输入
# 如果 /dev/tty 不可用（完全非交互环境），则要求用 UNINSTALL_YES=1 环境变量跳过确认
if [ -n "${UNINSTALL_YES:-}" ]; then
  echo -e "${YELLOW}[*] 检测到 UNINSTALL_YES=1，跳过确认直接执行${PLAIN}"
  CONFIRM="yes"
elif [ -t 0 ]; then
  # stdin 是终端（本地执行），直接 read
  read -rp "确认继续？输入 yes 回车执行，其他任意键退出: " CONFIRM
else
  # stdin 被管道占用（curl | bash），从 /dev/tty 读取键盘输入
  if [ -e /dev/tty ]; then
    read -rp "确认继续？输入 yes 回车执行，其他任意键退出: " CONFIRM </dev/tty
  else
    echo -e "${RED}[!] 非交互环境且无法打开 /dev/tty，请使用 UNINSTALL_YES=1 跳过确认：${PLAIN}"
    echo -e "${YELLOW}    UNINSTALL_YES=1 bash <(curl -fsSL ...)${PLAIN}"
    exit 1
  fi
fi

if [ "${CONFIRM}" != "yes" ]; then
  echo -e "${YELLOW}[*] 已取消${PLAIN}"
  exit 0
fi

# ── 步骤 1：停止并删除 Compose 管理的服务容器和镜像 ──────────────────────────
echo -e "\n${YELLOW}[1/4] 停止并删除面板容器组...${PLAIN}"
if [ "${SKIP_DIR_OPS}" = false ] && [ -f "${ROOT_DIR}/docker-compose.yml" ]; then
  COMPOSE_CMD=""
  if docker compose version >/dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
  elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE_CMD="docker-compose"
  fi

  if [ -n "${COMPOSE_CMD}" ]; then
    cd "${ROOT_DIR}"
    ${COMPOSE_CMD} down --rmi all --remove-orphans 2>/dev/null || true
    echo -e "${GREEN}[+] 面板容器组已清除${PLAIN}"
  else
    echo -e "${YELLOW}[!] 未找到 docker compose，跳过 Compose 清理${PLAIN}"
  fi
else
  # 无项目目录时，直接按名称停止已知容器
  for cname in vless-web-panel vless-self-healing-worker; do
    docker rm -f "${cname}" 2>/dev/null && echo -e "${GREEN}[+] 容器 ${cname} 已删除${PLAIN}" || true
  done
fi

# ── 步骤 2：强制删除所有 egress-* 出口容器 ───────────────────────────────────
echo -e "\n${YELLOW}[2/4] 清理所有出口容器 (egress-*)...${PLAIN}"
EGRESS_CONTAINERS=$(docker ps -a --format '{{.Names}}' 2>/dev/null | grep '^egress-' || true)
if [ -n "${EGRESS_CONTAINERS}" ]; then
  echo "${EGRESS_CONTAINERS}" | xargs docker rm -f
  EGRESS_COUNT=$(echo "${EGRESS_CONTAINERS}" | wc -l | tr -d ' ')
  echo -e "${GREEN}[+] 已删除 ${EGRESS_COUNT} 个出口容器${PLAIN}"
else
  echo -e "${GREEN}[+] 无出口容器需要清理${PLAIN}"
fi

# ── 步骤 3：删除出口基础镜像和构建缓存 ───────────────────────────────────────
echo -e "\n${YELLOW}[3/4] 删除出口基础镜像及构建缓存...${PLAIN}"
docker rmi vpngate-egress:latest 2>/dev/null \
  && echo -e "${GREEN}[+] vpngate-egress:latest 已删除${PLAIN}" \
  || echo -e "${YELLOW}[*] vpngate-egress:latest 不存在，跳过${PLAIN}"

docker image prune -f >/dev/null 2>&1 \
  && echo -e "${GREEN}[+] 孤儿镜像缓存已清理${PLAIN}"

# ── 步骤 4（可选）：彻底删除数据目录和项目目录 ──────────────────────────────
echo -e "\n${YELLOW}[4/4] 清理持久化数据...${PLAIN}"
if [ "${PURGE}" = true ] && [ "${SKIP_DIR_OPS}" = false ]; then
  rm -rf "${ROOT_DIR}/data"    && echo -e "${GREEN}[+] data/ 已删除${PLAIN}"
  rm -f  "${ROOT_DIR}/.env"    && echo -e "${GREEN}[+] .env 已删除${PLAIN}"
  rm -rf "${ROOT_DIR}/backups" && echo -e "${GREEN}[+] backups/ 已删除${PLAIN}"

  # 如果整个项目目录就是 clone 的仓库（存在 .git），则自删
  if [ -d "${ROOT_DIR}/.git" ]; then
    PARENT_DIR="$(dirname "${ROOT_DIR}")"
    PROJECT_DIR_NAME="$(basename "${ROOT_DIR}")"
    echo -e "${YELLOW}[*] 正在删除项目目录 ${ROOT_DIR} ...${PLAIN}"
    # 切出当前目录再删，避免 shell 报 cwd 不存在的警告
    cd "${PARENT_DIR}"
    rm -rf "${PROJECT_DIR_NAME}"
    echo -e "${GREEN}[+] 项目目录已彻底删除${PLAIN}"
  fi
elif [ "${PURGE}" = true ] && [ "${SKIP_DIR_OPS}" = true ]; then
  echo -e "${YELLOW}[*] 未找到项目目录，跳过数据目录清理${PLAIN}"
else
  echo -e "${YELLOW}[*] 已跳过（保留 data/ 和 .env，重新部署后数据可还原）${PLAIN}"
fi

# ── 完成 ──────────────────────────────────────────────────────────────────────
echo -e "\n${BLUE}====================================================${PLAIN}"
if [ "${PURGE}" = true ]; then
  echo -e "${GREEN}[+] 卸载完成，所有资源已彻底清除${PLAIN}"
else
  echo -e "${GREEN}[+] 卸载完成${PLAIN}"
  echo -e "${YELLOW}[*] 数据目录 data/ 和 .env 已保留，如需重新部署直接运行 deploy.sh${PLAIN}"
fi
echo -e "${BLUE}====================================================${PLAIN}"
