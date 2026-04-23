#!/usr/bin/env bash
set -euo pipefail

ADAPTER_BASE_URL="${ADAPTER_BASE_URL:-http://127.0.0.1:19091}"
EVENTS_ENDPOINT="${ADAPTER_BASE_URL}/feishu/events"
HEALTH_ENDPOINT="${ADAPTER_BASE_URL}/healthz"
METRICS_ENDPOINT="${ADAPTER_BASE_URL}/metrics"
CANCEL_ENDPOINT="${ADAPTER_BASE_URL}/feishu/actions/cancel"
PERMISSION_ENDPOINT="${ADAPTER_BASE_URL}/feishu/actions/permission"

echo "[drill] health check"
curl -sS "${HEALTH_ENDPOINT}" | jq .

echo "[drill] normal path: send message event"
NORMAL_EVENT='{
  "schema": "2.0",
  "header": {
    "event_id": "evt-drill-normal-1",
    "event_type": "im.message.receive_v1",
    "app_id": "cli-drill-app",
    "tenant_key": "tenant-drill"
  },
  "event": {
    "chat_id": "chat-drill",
    "thread_id": "thread-drill",
    "message": {
      "message_id": "msg-drill-1",
      "message_type": "text",
      "content": "{\"text\":\"hello from drill\"}"
    },
    "sender": {
      "sender_id": {
        "user_id": "user-drill"
      }
    }
  }
}'
curl -sS -X POST "${EVENTS_ENDPOINT}" \
  -H 'Content-Type: application/json' \
  -d "${NORMAL_EVENT}" | jq .

echo "[drill] boundary path: duplicate event_id should be deduped"
curl -sS -X POST "${EVENTS_ENDPOINT}" \
  -H 'Content-Type: application/json' \
  -d "${NORMAL_EVENT}" | jq .

echo "[drill] abnormal path: invalid payload"
curl -sS -X POST "${EVENTS_ENDPOINT}" \
  -H 'Content-Type: application/json' \
  -d '{"schema":"2.0","header":{"event_id":"evt-bad"},"event":{}}' | jq .

echo "[drill] cancel path: submit cancel callback"
curl -sS -X POST "${CANCEL_ENDPOINT}" \
  -H 'Content-Type: application/json' \
  -d '{"run_id":"run_fs_aaaaaaaaaaaaaaaaaaaa"}' | jq .

echo "[drill] permission path: simple callback"
curl -sS -X POST "${PERMISSION_ENDPOINT}" \
  -H 'Content-Type: application/json' \
  -d '{"request_id":"perm-drill-1","decision":"reject"}' | jq .

echo "[drill] permission path: card action callback"
curl -sS -X POST "${PERMISSION_ENDPOINT}" \
  -H 'Content-Type: application/json' \
  -d '{"event":{"action":{"value":{"request_id":"perm-drill-2","decision":"allow_once"}}}}' | jq .

echo "[drill] metrics snapshot"
curl -sS "${METRICS_ENDPOINT}" | jq .

echo "[drill] done"
