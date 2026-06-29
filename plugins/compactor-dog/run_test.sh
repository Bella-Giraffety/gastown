#!/usr/bin/env bash
# Tests for compactor-dog/run.sh helper functions.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FAILURES=0

# Source just the helper functions from run.sh by extracting them.
# We can't source the whole script (it runs immediately), so redefine here.
log() { echo "[test] $*"; }

# --- Copy validate_hash from run.sh (must stay in sync) ---
validate_hash() {
  local hash="$1"
  local context="$2"
  if [[ ! "$hash" =~ ^[a-v0-9]+$ ]]; then
    log "ERROR: Unsafe $context hash rejected: '$hash'"
    return 1
  fi
  return 0
}

# Verify our copy matches run.sh (guard against drift).
RUN_SH_REGEX=$(sed -n '/^validate_hash/,/^}/p' "$SCRIPT_DIR/run.sh" | grep -oP '\^\[.*\]\+\$')
TEST_REGEX=$(sed -n '/^validate_hash/,/^}/p' "$0" | grep -oP '\^\[.*\]\+\$')
if [[ "$RUN_SH_REGEX" != "$TEST_REGEX" ]]; then
  echo "FAIL: validate_hash regex in test ($TEST_REGEX) doesn't match run.sh ($RUN_SH_REGEX)"
  echo "      Update the test to match run.sh"
  exit 1
fi

HELPERS_FILE="$(mktemp)"
trap 'rm -f "$HELPERS_FILE"' EXIT
sed -n '/^resolve_dolt_host()/,/^}/p; /^resolve_dolt_port()/,/^}/p' "$SCRIPT_DIR/run.sh" >"$HELPERS_FILE"
source "$HELPERS_FILE"

assert_valid() {
  local hash="$1"
  if ! validate_hash "$hash" "test" >/dev/null 2>&1; then
    echo "FAIL: expected valid hash: '$hash'"
    FAILURES=$((FAILURES + 1))
  fi
}

assert_invalid() {
  local hash="$1"
  if validate_hash "$hash" "test" >/dev/null 2>&1; then
    echo "FAIL: expected invalid hash: '$hash'"
    FAILURES=$((FAILURES + 1))
  fi
}

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

# --- Tests ---

echo "=== validate_hash tests ==="

# Dolt base32 hashes (real examples)
assert_valid "aecqtmbdbabpalqnamq8atfv86ehjf7r"
assert_valid "0123456789abcdefghijklmnopqrstuv"
assert_valid "abc123"
assert_valid "00000000"

# Hex-only hashes should still pass (subset of base32)
assert_valid "deadbeef"
assert_valid "abcdef0123456789"

# Invalid: characters outside base32 range
assert_invalid "xyz"
assert_invalid "ABCDEF"
assert_invalid "hash-with-dashes"
assert_invalid "hash_with_underscores"
assert_invalid "hash with spaces"
assert_invalid ""
assert_invalid "../../../etc/passwd"
assert_invalid "'; DROP TABLE issues; --"

echo ""
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

echo ""
if [[ $FAILURES -gt 0 ]]; then
  echo "FAILED: $FAILURES test(s) failed"
  exit 1
else
  echo "PASSED: all tests passed"
fi
