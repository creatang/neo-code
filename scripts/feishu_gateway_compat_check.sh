#!/usr/bin/env bash
set -euo pipefail

GATEWAY_TARGETS="${GATEWAY_TARGETS:-ws://127.0.0.1:8080/ws,ws://127.0.0.1:18080/ws}"
GATEWAY_ORIGIN="${GATEWAY_ORIGIN:-http://localhost:3000}"
GATEWAY_TOKEN_FILE="${GATEWAY_TOKEN_FILE:-$HOME/.neocode/auth.json}"
REQUEST_TIMEOUT_SEC="${REQUEST_TIMEOUT_SEC:-8}"

if ! command -v neocode >/dev/null 2>&1; then
  echo "[compat] neocode command not found" >&2
  exit 1
fi

echo "[compat] running gateway compatibility check"
echo "[compat] targets: ${GATEWAY_TARGETS}"

neocode feishu-adapter \
  --compat-check \
  --gateway-origin "${GATEWAY_ORIGIN}" \
  --gateway-token-file "${GATEWAY_TOKEN_FILE}" \
  --request-timeout-sec "${REQUEST_TIMEOUT_SEC}" \
  --compat-targets "${GATEWAY_TARGETS}"

echo "[compat] passed"
