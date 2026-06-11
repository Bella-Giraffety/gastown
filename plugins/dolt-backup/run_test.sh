#!/usr/bin/env bash
# Black-box regression tests for dolt-backup/run.sh.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FAILURES=0
TEST_TMP_ROOT="$(mktemp -d)"

cleanup() {
  rm -rf "$TEST_TMP_ROOT"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*"
  FAILURES=$((FAILURES + 1))
}

assert_file_exists() {
  local path="$1"
  [[ -e "$path" ]] || fail "expected file to exist: $path"
}

assert_file_missing() {
  local path="$1"
  [[ ! -e "$path" ]] || fail "expected file to be missing: $path"
}

assert_contains() {
  local path="$1"
  local text="$2"
  grep -Fq -- "$text" "$path" || fail "expected $path to contain: $text"
}

assert_not_contains() {
  local path="$1"
  local text="$2"
  if [[ -e "$path" ]] && grep -Fq -- "$text" "$path"; then
    fail "expected $path not to contain: $text"
  fi
}

write_stubs() {
  local bin_dir="$1"

  cat > "$bin_dir/dolt" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail

DB="$(basename "$PWD")"
REMOTE_FILE="$DOLT_STUB_ROOT/remotes/$DB"
ADD_LOG="$DOLT_STUB_ROOT/add.log"
SYNC_LOG="$DOLT_STUB_ROOT/sync.log"
OPS_LOG="$DOLT_STUB_ROOT/ops.log"

case "${1:-}" in
  log)
    echo "${DOLT_STUB_HASH:-hash1} commit message"
    ;;
  backup)
    case "${2:-}" in
      -v|--verbose|"")
        if [[ -f "$REMOTE_FILE" ]]; then
          while IFS= read -r remote; do
            [[ -n "$remote" ]] && echo "$remote file://example/$remote"
          done < "$REMOTE_FILE"
        fi
        ;;
      add)
        echo "backup add $3 $4" >> "$ADD_LOG"
        echo "add $3 $4" >> "$OPS_LOG"
        if [[ "${DOLT_STUB_ADD_FAIL:-0}" = "1" ]]; then
          echo "add failed"
          exit 17
        fi
        mkdir -p "$(dirname "$REMOTE_FILE")"
        echo "$3" >> "$REMOTE_FILE"
        ;;
      sync)
        echo "backup sync $3" >> "$SYNC_LOG"
        echo "sync $3" >> "$OPS_LOG"
        if [[ "${DOLT_STUB_SYNC_FAIL:-0}" = "1" ]]; then
          echo "sync failed"
          exit 42
        fi
        mkdir -p "$DOLT_BACKUP_DIR/$DB/$3"
        echo "payload" > "$DOLT_BACKUP_DIR/$DB/$3/data"
        ;;
      *)
        echo "unexpected backup command: $*" >&2
        exit 2
        ;;
    esac
    ;;
  *)
    echo "unexpected dolt command: $*" >&2
    exit 2
    ;;
esac
STUB

  cat > "$bin_dir/bd" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB

  cat > "$bin_dir/gt" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB

  chmod +x "$bin_dir/dolt" "$bin_dir/bd" "$bin_dir/gt"
}

setup_case() {
  local tmp
  tmp="$(mktemp -d "$TEST_TMP_ROOT/case.XXXXXX")"
  mkdir -p "$tmp/bin" "$tmp/data/hq/.dolt" "$tmp/backup" "$tmp/remotes"
  write_stubs "$tmp/bin"
  echo "$tmp"
}

run_backup() {
  local tmp="$1"
  shift

  env \
    PATH="$tmp/bin:$PATH" \
    DOLT_DATA_DIR="$tmp/data" \
    DOLT_BACKUP_DIR="$tmp/backup" \
    DOLT_STUB_ROOT="$tmp" \
    "$@" \
    "$SCRIPT_DIR/run.sh" --databases hq > "$tmp/output.log" 2>&1
}

test_failed_sync_does_not_write_hash() {
  local tmp
  tmp="$(setup_case)"
  if run_backup "$tmp" DOLT_STUB_SYNC_FAIL=1; then
    fail "failed sync should exit nonzero"
  fi
  assert_file_missing "$tmp/backup/hq/.last-backup-hash"
  assert_contains "$tmp/sync.log" "backup sync hq-backup"
  assert_contains "$tmp/output.log" "FAILED (exit 42)"
  rm -rf "$tmp"
}

test_missing_remote_is_added_before_sync() {
  local tmp before after
  tmp="$(setup_case)"
  mkdir -p "$tmp/backup/hq"
  touch -d '2000-01-01 UTC' "$tmp/backup/hq"
  before="$(stat -c %Y "$tmp/backup/hq")"
  if ! run_backup "$tmp"; then
    fail "missing remote case should succeed"
  fi
  after="$(stat -c %Y "$tmp/backup/hq")"
  assert_contains "$tmp/add.log" "backup add hq-backup file://$tmp/backup/hq/hq-backup"
  assert_contains "$tmp/sync.log" "backup sync hq-backup"
  mapfile -t ops < "$tmp/ops.log"
  [[ "${ops[0]:-}" = "add hq-backup file://$tmp/backup/hq/hq-backup" ]] || fail "backup add should be first operation"
  [[ "${ops[1]:-}" = "sync hq-backup" ]] || fail "backup sync should follow backup add"
  assert_contains "$tmp/backup/hq/.last-backup-hash" "hash1"
  assert_file_exists "$tmp/backup/hq/hq-backup/data"
  [[ "$after" -gt "$before" ]] || fail "successful sync should touch backup dir mtime"
  rm -rf "$tmp"
}

test_stale_hash_without_backup_forces_sync() {
  local tmp
  tmp="$(setup_case)"
  mkdir -p "$tmp/backup/hq/hq-backup"
  echo "hash1" > "$tmp/backup/hq/.last-backup-hash"
  echo "hq-backup" > "$tmp/remotes/hq"
  if ! run_backup "$tmp"; then
    fail "stale hash repair should succeed"
  fi
  assert_file_missing "$tmp/add.log"
  assert_contains "$tmp/sync.log" "backup sync hq-backup"
  assert_file_exists "$tmp/backup/hq/hq-backup/data"
  assert_contains "$tmp/backup/hq/.last-backup-hash" "hash1"
  rm -rf "$tmp"
}

test_missing_databases_value_errors() {
  local tmp
  tmp="$(setup_case)"
  if env \
    PATH="$tmp/bin:$PATH" \
    DOLT_DATA_DIR="$tmp/data" \
    DOLT_BACKUP_DIR="$tmp/backup" \
    DOLT_STUB_ROOT="$tmp" \
    "$SCRIPT_DIR/run.sh" --databases > "$tmp/output.log" 2>&1; then
    fail "missing --databases value should exit nonzero"
  fi
  assert_contains "$tmp/output.log" "ERROR: --databases requires a comma-separated value"
  rm -rf "$tmp"
}

test_add_remote_failure_does_not_sync_or_write_hash() {
  local tmp
  tmp="$(setup_case)"
  if run_backup "$tmp" DOLT_STUB_ADD_FAIL=1; then
    fail "add remote failure should exit nonzero"
  fi
  assert_contains "$tmp/add.log" "backup add hq-backup file://$tmp/backup/hq/hq-backup"
  assert_file_missing "$tmp/sync.log"
  assert_file_missing "$tmp/backup/hq/.last-backup-hash"
  assert_contains "$tmp/output.log" "FAILED to add backup remote"
  rm -rf "$tmp"
}

test_unchanged_real_backup_skips_and_touches() {
  local tmp before after
  tmp="$(setup_case)"
  mkdir -p "$tmp/backup/hq/hq-backup"
  echo "payload" > "$tmp/backup/hq/hq-backup/data"
  echo "hash1" > "$tmp/backup/hq/.last-backup-hash"
  echo "hq-backup" > "$tmp/remotes/hq"
  touch -d '2000-01-01 UTC' "$tmp/backup/hq"
  before="$(stat -c %Y "$tmp/backup/hq")"
  if ! run_backup "$tmp"; then
    fail "unchanged real backup should succeed"
  fi
  after="$(stat -c %Y "$tmp/backup/hq")"
  assert_not_contains "$tmp/add.log" "backup add"
  assert_file_missing "$tmp/sync.log"
  assert_contains "$tmp/output.log" "unchanged (hash1), skipping"
  [[ "$after" -gt "$before" ]] || fail "unchanged skip should touch backup dir mtime"
  rm -rf "$tmp"
}

echo "=== dolt-backup tests ==="
test_failed_sync_does_not_write_hash
test_missing_remote_is_added_before_sync
test_stale_hash_without_backup_forces_sync
test_add_remote_failure_does_not_sync_or_write_hash
test_unchanged_real_backup_skips_and_touches
test_missing_databases_value_errors

if [[ $FAILURES -gt 0 ]]; then
  echo "FAILED: $FAILURES test(s) failed"
  exit 1
fi

echo "PASSED: all tests passed"
