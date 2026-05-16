#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

cleanup() {
  docker compose down --remove-orphans >/dev/null 2>&1 || true
}

tmp_file="$(mktemp)"
trap 'rm -f "$tmp_file"; cleanup' EXIT

echo "[rewriter-e2e] starting docker compose stack"
docker compose up -d --build

echo "[rewriter-e2e] waiting for envoy readiness"
for _ in $(seq 1 120); do
  if curl -fsS http://localhost:9901/ready >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! curl -fsS http://localhost:9901/ready >/dev/null 2>&1; then
  echo "[rewriter-e2e] envoy is not ready after timeout" >&2
  exit 1
fi

echo "[rewriter-e2e] generating tokens"
tokens_output="$(cd authz-server && go run cmd/gen-token/main.go)"
user_token="$(printf '%s\n' "$tokens_output" | awk '/^=== User Token/{getline; print; exit}')"
admin_token="$(printf '%s\n' "$tokens_output" | awk '/^=== Admin Token/{getline; print; exit}')"

if [ -z "$user_token" ]; then
  echo "[rewriter-e2e] failed to parse user token" >&2
  exit 1
fi

PASS=0
FAIL=0

assert_status() {
  local test_name="$1" expected="$2" actual="$3"
  if [ "$actual" = "$expected" ]; then
    echo "[PASS] $test_name (status=$actual)"
    PASS=$((PASS + 1))
  else
    echo "[FAIL] $test_name: expected status $expected, got $actual" >&2
    cat "$tmp_file" >&2
    FAIL=$((FAIL + 1))
  fi
}

# Wait for downstream service readiness
echo "[rewriter-e2e] waiting for graphql service"
for _ in $(seq 1 60); do
  status="$(curl -sS -o /dev/null -w '%{http_code}' -X POST http://localhost:10000/query \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${admin_token}" \
    -d '{"query":"{ employeeByID(id:\"emp-1\") { id } }"}' 2>/dev/null || true)"
  if [ "$status" = "200" ]; then
    break
  fi
  sleep 1
done

# --- Test 1: Normal rewrite (salary removed for user) ---
echo "[rewriter-e2e] Test 1: normal field rewrite"
status="$(curl -sS -o "$tmp_file" -w '%{http_code}' -X POST http://localhost:10000/query \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${user_token}" \
  -d '{"query":"{ employeeByID(id:\"emp-1\") { id name salary } }"}')"
assert_status "normal rewrite returns 200" "200" "$status"
if grep -q "salary" "$tmp_file"; then
  echo "[FAIL] salary should not appear in response" >&2
  FAIL=$((FAIL + 1))
else
  echo "[PASS] salary field removed from response"
  PASS=$((PASS + 1))
fi

# --- Test 2: Empty query denied (only salary requested) ---
echo "[rewriter-e2e] Test 2: empty query after rewrite"
status="$(curl -sS -o "$tmp_file" -w '%{http_code}' -X POST http://localhost:10000/query \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${user_token}" \
  -d '{"query":"{ employeeByID(id:\"emp-1\") { salary } }"}')"
assert_status "empty query returns 403" "403" "$status"

# --- Test 3: APQ bypass denied ---
echo "[rewriter-e2e] Test 3: APQ bypass"
status="$(curl -sS -o "$tmp_file" -w '%{http_code}' -X POST http://localhost:10000/query \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${user_token}" \
  -d '{"extensions":{"persistedQuery":{"sha256Hash":"abc123"}}}')"
assert_status "APQ bypass returns 403" "403" "$status"

# --- Test 4: GET request denied ---
echo "[rewriter-e2e] Test 4: GET request bypass"
status="$(curl -sS -o "$tmp_file" -w '%{http_code}' -G http://localhost:10000/query \
  --data-urlencode 'query={ employeeByID(id:"emp-1") { id salary } }' \
  -H "Authorization: Bearer ${user_token}")"
assert_status "GET request returns 403" "403" "$status"

# --- Test 5: Admin can access salary ---
echo "[rewriter-e2e] Test 5: admin access (no rewrite)"
status="$(curl -sS -o "$tmp_file" -w '%{http_code}' -X POST http://localhost:10000/query \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${admin_token}" \
  -d '{"query":"{ employeeByID(id:\"emp-1\") { id name salary } }"}')"
assert_status "admin returns 200" "200" "$status"
if ! grep -q "salary" "$tmp_file"; then
  echo "[FAIL] admin should see salary in response" >&2
  FAIL=$((FAIL + 1))
else
  echo "[PASS] admin sees salary field"
  PASS=$((PASS + 1))
fi

# --- Summary ---
echo ""
echo "[rewriter-e2e] Results: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
echo "[rewriter-e2e] ALL TESTS PASSED"
