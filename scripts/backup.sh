#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKUP_DIR="${BACKUP_DIR:-${ROOT_DIR}/backups}"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUT_FILE="${BACKUP_DIR}/vless-reality-backup-${STAMP}.tar.gz"

mkdir -p "${BACKUP_DIR}"

cd "${ROOT_DIR}"

INCLUDES=()
[ -d data ] && INCLUDES+=("data")
[ -f .env ] && INCLUDES+=(".env")

if [ "${#INCLUDES[@]}" -eq 0 ]; then
  echo "[!] No data or .env found to back up."
  exit 1
fi

tar -czf "${OUT_FILE}" "${INCLUDES[@]}"
chmod 600 "${OUT_FILE}"

echo "${OUT_FILE}"

# 保留最近 N 份备份，自动清理旧文件（默认 7 份，可通过 BACKUP_KEEP 覆盖）
BACKUP_KEEP="${BACKUP_KEEP:-7}"
OLD_BACKUPS=$(ls -1t "${BACKUP_DIR}"/vless-reality-backup-*.tar.gz 2>/dev/null | tail -n +"$((BACKUP_KEEP + 1))")
if [ -n "${OLD_BACKUPS}" ]; then
  echo "${OLD_BACKUPS}" | xargs rm -f
  echo "[*] 已清理超出保留策略的旧备份（保留最近 ${BACKUP_KEEP} 份）"
fi
