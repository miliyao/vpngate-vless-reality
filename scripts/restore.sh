#!/bin/bash

set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 /path/to/vless-reality-backup.tar.gz"
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKUP_FILE="$1"

if [ ! -f "${BACKUP_FILE}" ]; then
  echo "[!] Backup file not found: ${BACKUP_FILE}"
  exit 1
fi

cd "${ROOT_DIR}"

echo "[*] Stopping services..."
docker compose down

echo "[*] Restoring ${BACKUP_FILE}..."
tar -xzf "${BACKUP_FILE}" -C "${ROOT_DIR}"

[ -f .env ] && chmod 600 .env

echo "[*] Starting services..."
docker compose up -d

echo "[+] Restore complete."
