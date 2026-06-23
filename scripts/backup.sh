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
