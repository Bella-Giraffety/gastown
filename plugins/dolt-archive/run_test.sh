#!/usr/bin/env bash
# Tests for dolt-archive/run.sh endpoint resolution helpers.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FAILURES=0

HELPERS_FILE="$(mktemp)"
trap 'rm -f "$HELPERS_FILE"' EXIT
sed -n '/^resolve_dolt_host()/,/^}/p; /^resolve_dolt_port()/,/^}/p' "$SCRIPT_DIR/run.sh" >"$HELPERS_FILE"
source "$HELPERS_FILE"

reset_endpoint_env() {
  unset GT_DOLT_HOST BEADS_DOLT_SERVER_HOST DOLT_HOST
  unset GT_DOLT_PORT BEADS_DOLT_SERVER_PORT BEADS_DOLT_PORT DOLT_PORT
}

assert_eq() {
  local got="$1" want="$2" name="$3"
  if [[ "$got" != "$want" ]]; then
    echo "FAIL: $name: got '$got', want '$want'"
    FAILURES=$((FAILURES + 1))
  fi
}

echo "=== endpoint resolution tests ==="

reset_endpoint_env
assert_eq "$(resolve_dolt_host)" "127.0.0.1" "default host"
assert_eq "$(resolve_dolt_port)" "3307" "default port"

reset_endpoint_env
BEADS_DOLT_SERVER_HOST="10.0.0.3"
BEADS_DOLT_SERVER_PORT="4417"
assert_eq "$(resolve_dolt_host)" "10.0.0.3" "beads host fallback"
assert_eq "$(resolve_dolt_port)" "4417" "beads server port fallback"

reset_endpoint_env
GT_DOLT_HOST="10.0.0.2"
BEADS_DOLT_SERVER_HOST="10.0.0.3"
GT_DOLT_PORT="5507"
BEADS_DOLT_SERVER_PORT="4417"
assert_eq "$(resolve_dolt_host)" "10.0.0.2" "gt host wins"
assert_eq "$(resolve_dolt_port)" "5507" "gt port wins"

reset_endpoint_env
BEADS_DOLT_PORT="4408"
DOLT_PORT="3309"
assert_eq "$(resolve_dolt_port)" "4408" "legacy beads port before plugin port"

reset_endpoint_env
DOLT_HOST="10.0.0.1"
DOLT_PORT="3309"
assert_eq "$(resolve_dolt_host)" "10.0.0.1" "plugin host fallback"
assert_eq "$(resolve_dolt_port)" "3309" "plugin port fallback"

reset_endpoint_env
GT_DOLT_PORT="not-a-port"
if (resolve_dolt_port >/dev/null 2>&1); then
  echo "FAIL: invalid port should be rejected"
  FAILURES=$((FAILURES + 1))
fi

reset_endpoint_env
GT_DOLT_PORT="70000"
if (resolve_dolt_port >/dev/null 2>&1); then
  echo "FAIL: out-of-range port should be rejected"
  FAILURES=$((FAILURES + 1))
fi

if [[ $FAILURES -gt 0 ]]; then
  echo "FAILED: $FAILURES test(s) failed"
  exit 1
fi

echo "PASSED: all tests passed"
