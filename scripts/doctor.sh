#!/bin/bash

set -u

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

PASS=0
FAIL=0

ok() {
  PASS=$((PASS + 1))
  echo "[OK] $1"
}

fail() {
  FAIL=$((FAIL + 1))
  echo "[FAIL] $1"
}

check_cmd() {
  if command -v "$1" >/dev/null 2>&1; then
    ok "$1 is installed"
  else
    fail "$1 is not installed"
  fi
}

load_env() {
  if [ -f .env ]; then
    set -a
    . ./.env
    set +a
    ok ".env loaded"
  else
    fail ".env is missing"
  fi
}

check_file() {
  if [ -e "$1" ]; then
    ok "$1 exists"
  else
    fail "$1 is missing"
  fi
}

check_http() {
  local name="$1"
  local expected="$2"
  shift 2
  local code
  code="$(curl -s -o /tmp/vless-doctor-http.out -w '%{http_code}' "$@" || true)"
  if [ "${code}" = "${expected}" ]; then
    ok "${name} returned ${code}"
  else
    fail "${name} returned ${code}, expected ${expected}"
  fi
}

echo "== VLESS Reality Panel Doctor =="

check_cmd docker
check_cmd curl

if docker compose version >/dev/null 2>&1; then
  ok "docker compose is available"
else
  fail "docker compose is not available"
fi

load_env

check_file docker-compose.yml
check_file config/xray-config.template.json
check_file data
check_file web/backend/dist/index.html

if docker image inspect vpngate-egress:latest >/dev/null 2>&1; then
  ok "vpngate-egress:latest image exists"
else
  fail "vpngate-egress:latest image is missing"
fi

if docker compose config >/tmp/vless-doctor-compose.yml 2>/tmp/vless-doctor-compose.err; then
  ok "docker compose config is valid"
else
  fail "docker compose config is invalid"
fi

PANEL_PORT="${PANEL_PORT:-3000}"
PANEL_USERNAME="${PANEL_USERNAME:-admin}"
BASE_URL="http://127.0.0.1:${PANEL_PORT}"

check_http "/healthz" "200" "${BASE_URL}/healthz"

if [ -n "${PANEL_PASSWORD:-}" ]; then
  check_http "/api/system/status auth" "200" -u "${PANEL_USERNAME}:${PANEL_PASSWORD}" "${BASE_URL}/api/system/status"
  check_http "/api/system/status unauth" "401" "${BASE_URL}/api/system/status"
else
  fail "PANEL_PASSWORD is not set"
fi

for container in vless-web-panel vless-self-healing-worker; do
  if docker inspect "${container}" >/dev/null 2>&1; then
    status="$(docker inspect --format '{{.State.Status}}' "${container}")"
    health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "${container}")"
    if [ "${status}" = "running" ] && { [ "${health}" = "healthy" ] || [ "${health}" = "none" ]; }; then
      ok "${container} is ${status}/${health}"
    else
      fail "${container} is ${status}/${health}"
    fi
  else
    fail "${container} does not exist"
  fi
done

echo "== Result: ${PASS} passed, ${FAIL} failed =="

if [ "${FAIL}" -gt 0 ]; then
  exit 1
fi
