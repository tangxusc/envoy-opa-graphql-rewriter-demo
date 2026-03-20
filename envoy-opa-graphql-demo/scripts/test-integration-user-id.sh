#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

cleanup() {
  docker compose down --remove-orphans >/dev/null 2>&1 || true
}

denied_body_file="$(mktemp)"
authorized_body_file="$(mktemp)"
trap 'rm -f "$denied_body_file" "$authorized_body_file"; cleanup' EXIT

echo "[integration] starting docker compose stack"
docker compose up -d --build

echo "[integration] waiting for envoy readiness"
for _ in $(seq 1 120); do
  if curl -fsS http://localhost:9901/ready >/dev/null; then
    break
  fi
  sleep 1
done

if ! curl -fsS http://localhost:9901/ready >/dev/null; then
  echo "[integration] envoy is not ready after timeout" >&2
  exit 1
fi

echo "[integration] generating user token"
tokens_output="$(cd authz-server && go run cmd/gen-token/main.go)"
user_token="$(printf '%s\n' "$tokens_output" | awk '/^=== User Token/{getline; print; exit}')"
if [ -z "$user_token" ]; then
  echo "[integration] failed to parse user token" >&2
  exit 1
fi

graphql_logs() {
  docker compose logs --no-color graphql-server 2>&1 || true
}

count_user_logs() {
  graphql_logs | grep -c 'x-user-id=user-1' || true
}

query_payload='{"query":"{ employeeByID(id:\"emp-1\") { id name } }"}'

initial_user_logs="$(count_user_logs)"

echo "[integration] sending authorized request through envoy"
authorized_status=""
authorized_response=""
for _ in $(seq 1 60); do
  authorized_status="$(curl -sS -o "$authorized_body_file" -w '%{http_code}' -X POST http://localhost:10000/query \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${user_token}" \
    -d "$query_payload" || true)"
  authorized_response="$(cat "$authorized_body_file")"
  if [ "$authorized_status" = "200" ] && printf '%s' "$authorized_response" | grep -q '"employeeByID"'; then
    break
  fi
  sleep 1
done

if [ "$authorized_status" != "200" ] || ! printf '%s' "$authorized_response" | grep -q '"employeeByID"'; then
  echo "[integration] authorized response missing expected data (status=$authorized_status): $authorized_response" >&2
  exit 1
fi

echo "[integration] checking downstream graphql logs for forwarded x-user-id"
found=0
for _ in $(seq 1 30); do
  if graphql_logs | grep -q 'x-user-id=user-1'; then
    found=1
    break
  fi
  sleep 1
done
if [ "$found" -ne 1 ]; then
  echo "[integration] missing downstream log x-user-id=user-1" >&2
  graphql_logs | tail -n 100 >&2
  exit 1
fi

user_logs_after_auth="$(count_user_logs)"
if [ "$user_logs_after_auth" -le "$initial_user_logs" ]; then
  echo "[integration] expected additional x-user-id log after authorized request" >&2
  exit 1
fi

echo "[integration] sending unauthorized request (should be rejected before downstream)"
denied_status="$(curl -sS -o "$denied_body_file" -w '%{http_code}' -X POST http://localhost:10000/query \
  -H 'Content-Type: application/json' \
  -d "$query_payload")"

if [ "$denied_status" != "403" ] && [ "$denied_status" != "401" ]; then
  echo "[integration] expected 401/403 for unauthorized request, got $denied_status" >&2
  cat "$denied_body_file" >&2
  exit 1
fi

sleep 2
user_logs_after_denied="$(count_user_logs)"
if [ "$user_logs_after_denied" -ne "$user_logs_after_auth" ]; then
  echo "[integration] unauthorized request appears to have reached graphql-server" >&2
  echo "before denied: $user_logs_after_auth, after denied: $user_logs_after_denied" >&2
  exit 1
fi

echo "[integration] PASS: x-user-id is injected and forwarded downstream"
