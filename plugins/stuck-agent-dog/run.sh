#!/usr/bin/env bash
# stuck-agent-dog/run.sh — Context-aware stuck/crashed agent detection.
#
# SCOPE: Only polecats and deacon. NEVER touches crew, mayor, witness, or refinery.
# The daemon detects; this plugin inspects context before acting.

set -euo pipefail

log() { echo "[stuck-agent-dog] $*"; }

TOWN_ROOT="${GT_TOWN_ROOT:-}"
if [ -z "$TOWN_ROOT" ]; then
  if ! TOWN_ROOT=$(gt town root 2>/dev/null); then
    log "SKIP: could not resolve town root"
    exit 0
  fi
fi

RIGS_JSON_PATH="${TOWN_ROOT}/rigs.json"
if [ ! -f "$RIGS_JSON_PATH" ] && [ -f "$TOWN_ROOT/mayor/rigs.json" ]; then
  RIGS_JSON_PATH="$TOWN_ROOT/mayor/rigs.json"
fi

integer_or_default() {
  local value="$1"
  local default="$2"

  case "$value" in
    ''|*[!0-9]*) echo "$default" ;;
    *) echo "$value" ;;
  esac
}

positive_integer_or_default() {
  local value="$1"
  local default="$2"

  case "$value" in
    ''|*[!0-9]*) echo "$default" ;;
    *)
      if [ "$value" -ge 1 ]; then
        echo "$value"
      else
        echo "$default"
      fi
      ;;
  esac
}

POLECAT_MAX_INACTIVITY="${GT_STUCK_AGENT_DOG_MAX_INACTIVITY:-0s}"
[ "$POLECAT_MAX_INACTIVITY" = "0" ] && POLECAT_MAX_INACTIVITY="0s"
DEACON_STALE_SECONDS=$(integer_or_default "${GT_STUCK_AGENT_DOG_DEACON_STALE_SECONDS:-}" 1200)
MASS_DEATH_THRESHOLD=$(positive_integer_or_default "${GT_STUCK_AGENT_DOG_MASS_DEATH_THRESHOLD:-}" 3)

heartbeat_epoch() {
  local file="$1"
  local ts=""

  ts=$(jq -r '(.timestamp // empty) | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601? // empty' "$file" 2>/dev/null || true)
  if [ -n "$ts" ]; then
    echo "$ts"
    return 0
  fi

  # Fallback for malformed legacy files: use mtime rather than failing open.
  # GNU stat (-c %Y) first: on GNU, 'stat -f' is filesystem mode and dumps a
  # multi-line "File: ..." block to stdout BEFORE failing, polluting the
  # command substitution and breaking downstream arithmetic (hq-wisp-0vrp).
  # BSD/macOS stat (-f %m) is the fallback.
  stat -c %Y "$file" 2>/dev/null || stat -f %m "$file" 2>/dev/null
}

# --- Beads resolution helpers -------------------------------------------------
# Plugin scripts may run outside a beads workspace. Resolve hook and status
# lookups from the target rig workspace, and make missing/inactive rigs
# non-fatal so one bad rig does not abort the dog under `set -e` (hq-9e770).

rig_workdir() {
  local rig="$1"

  if [ -d "$TOWN_ROOT/$rig/mayor/rig" ]; then
    printf '%s\n' "$TOWN_ROOT/$rig/mayor/rig"
    return 0
  fi

  if [ -d "$TOWN_ROOT/$rig" ]; then
    printf '%s\n' "$TOWN_ROOT/$rig"
    return 0
  fi

  return 1
}

rig_hook_assignment() {
  local rig="$1" pcat="$2" dir=""
  local hook_json="" bead="" status=""

  if ! dir=$(rig_workdir "$rig"); then
    return 0
  fi

  hook_json=$( ( cd "$dir" 2>/dev/null && gt hook show "$rig/polecats/$pcat" --json 2>/dev/null ) || true )
  if [ -z "$hook_json" ]; then
    return 0
  fi

  bead=$(printf '%s' "$hook_json" | jq -r '.bead_id // empty' 2>/dev/null || true)
  status=$(printf '%s' "$hook_json" | jq -r '.status // empty' 2>/dev/null || true)
  [ -n "$bead" ] || return 0

  printf '%s|%s\n' "$bead" "$status"
}

hook_restartable() {
  local session="$1" bead="$2" status="$3"

  case "$status" in
    hooked|in_progress) [ -n "$bead" ] && return 0 ;;
    empty|"") log "  SKIP $session: no active hook" ;;
    *) log "  SKIP $session: hook=$bead status=$status not actionable" ;;
  esac

  return 1
}

session_health_status() {
  local session_name="$1"
  local health_json=""
  local status=""

  health_json=$(gt session health "$session_name" --json --max-inactivity "$POLECAT_MAX_INACTIVITY" 2>/dev/null) || return 1
  status=$(printf '%s' "$health_json" | jq -r '.status // empty' 2>/dev/null || true)
  [ -n "$status" ] || return 1
  printf '%s\n' "$status"
}

operational_rig_prefix_map() {
  local rig_json="" rows=""

  if rig_json=$(gt rig list --json 2>/dev/null); then
    if rows=$(printf '%s' "$rig_json" | jq -r '
      if type == "array" then
        .[]
        | select((.status // "operational") == "operational")
        | "\(.name)|\(.beads_prefix // .prefix // .beads.prefix // empty)"
      else
        error("expected array")
      end
    ' 2>/dev/null); then
      printf '%s\n' "$rows" | awk -F'|' 'NF >= 2 && $1 != "" && $2 != ""'
      return 0
    fi

    log "WARN: gt rig list --json was not parseable; falling back to rigs.json"
  fi

  # Fallback for older/runtime-copied layouts where gt rig list is unavailable.
  if [ ! -f "$RIGS_JSON_PATH" ]; then
    log "SKIP: rigs.json not found"
    return 0
  fi

  if ! rows=$(jq -r '
    if (.rigs | type) == "object" then
      .rigs | to_entries[] | "\(.key)|\(.value.beads.prefix // .key)"
    else
      empty
    end
  ' "$RIGS_JSON_PATH" 2>/dev/null); then
    log "SKIP: could not parse rigs.json"
    return 0
  fi

  printf '%s\n' "$rows" | awk -F'|' 'NF >= 2 && $1 != "" && $2 != ""'
}

check_control_plane() {
  local rig="$1" prefix="$2"
  local role="" session="" status=""

  for role in witness refinery; do
    session="${prefix}-${role}"
    status=$(session_health_status "$session" || true)
    case "$status" in
      healthy|agent-hung|agent_hung)
        ;;
      agent-dead|agent_dead|session-dead|session_dead)
        CONTROL_PLANE_OUTAGES+=("$session|$rig|$role|$status")
        log "  OUTAGE: $session ($status)"
        ;;
      *)
        log "  SKIP $session: control-plane liveness probe inconclusive"
        ;;
    esac
  done
}

confirm_polecat_outages() {
  local entry="" session="" rig="" pcat="" hook="" reason=""
  local health_status="" hook_assignment="" hook_bead="" hook_status=""

  CONFIRMED_CRASHED=()
  CONFIRMED_STUCK=()

  for entry in ${CRASHED[@]+"${CRASHED[@]}"}; do
    IFS='|' read -r session rig pcat hook <<< "$entry"
    health_status=$(session_health_status "$session" || true)
    if [ "$health_status" != "session-dead" ] && [ "$health_status" != "session_dead" ]; then
      log "  NOTICE: $session recovered before mass-death escalation (health=$health_status)"
      continue
    fi
    hook_assignment=$(rig_hook_assignment "$rig" "$pcat")
    IFS='|' read -r hook_bead hook_status <<< "$hook_assignment"
    if hook_restartable "$session" "$hook_bead" "$hook_status"; then
      CONFIRMED_CRASHED+=("$session|$rig|$pcat|$hook_bead")
    fi
  done

  for entry in ${STUCK[@]+"${STUCK[@]}"}; do
    IFS='|' read -r session rig pcat hook reason <<< "$entry"
    health_status=$(session_health_status "$session" || true)
    if [ "$health_status" != "agent-dead" ] && [ "$health_status" != "agent_dead" ]; then
      log "  NOTICE: $session recovered before mass-death escalation (health=$health_status)"
      continue
    fi
    hook_assignment=$(rig_hook_assignment "$rig" "$pcat")
    IFS='|' read -r hook_bead hook_status <<< "$hook_assignment"
    if hook_restartable "$session" "$hook_bead" "$hook_status"; then
      CONFIRMED_STUCK+=("$session|$rig|$pcat|$hook_bead|$reason")
    fi
  done
}

mass_death_fingerprint() {
  {
    local entry="" session="" rig="" pcat="" hook="" reason=""
    for entry in ${CRASHED[@]+"${CRASHED[@]}"}; do
      IFS='|' read -r session rig pcat hook <<< "$entry"
      printf '%s|session-dead\n' "$session"
    done
    for entry in ${STUCK[@]+"${STUCK[@]}"}; do
      IFS='|' read -r session rig pcat hook reason <<< "$entry"
      printf '%s|agent-dead\n' "$session"
    done
  } | LC_ALL=C sort | tr '\n' ';'
}

# --- Enumerate agents ---------------------------------------------------------

log "=== Checking agent health ==="

# Build operational rig_name|prefix mapping. The gt rig registry is the
# authoritative dock/park filter; raw rigs.json is only a degraded fallback.
RIG_PREFIX_MAP=$(operational_rig_prefix_map)
if [ -z "$RIG_PREFIX_MAP" ]; then
  log "SKIP: no operational rigs found"
  exit 0
fi

# --- Check polecat health ----------------------------------------------------

CRASHED=()
STUCK=()
CONTROL_PLANE_OUTAGES=()
HEALTHY=0

while IFS='|' read -r RIG PREFIX; do
  [ -z "$RIG" ] && continue
  check_control_plane "$RIG" "$PREFIX"

  POLECAT_DIR="$TOWN_ROOT/$RIG/polecats"
  [ -d "$POLECAT_DIR" ] || continue

  for PCAT_PATH in "$POLECAT_DIR"/*/; do
    [ -d "$PCAT_PATH" ] || continue
    PCAT_NAME=$(basename "$PCAT_PATH")
    SESSION_NAME="${PREFIX}-${PCAT_NAME}"

    HEALTH_STATUS=$(session_health_status "$SESSION_NAME" || true)
    case "$HEALTH_STATUS" in
      healthy)
        HEALTHY=$((HEALTHY + 1))
        ;;
      agent-dead|agent_dead)
        HOOK_ASSIGNMENT=$(rig_hook_assignment "$RIG" "$PCAT_NAME")
        IFS='|' read -r HOOK_BEAD HOOK_STATUS <<< "$HOOK_ASSIGNMENT"
        if hook_restartable "$SESSION_NAME" "$HOOK_BEAD" "$HOOK_STATUS"; then
          STUCK+=("$SESSION_NAME|$RIG|$PCAT_NAME|$HOOK_BEAD|agent_dead")
          log "  ZOMBIE: $SESSION_NAME (agent runtime dead, hook=$HOOK_BEAD)"
        fi
        ;;
      agent-hung|agent_hung)
        # A live runtime with quiet output can be a long research turn. Do not
        # kill it here; operators can tune the threshold and inspect manually.
        HEALTHY=$((HEALTHY + 1))
        log "  OBSERVE: $SESSION_NAME runtime alive but inactive beyond $POLECAT_MAX_INACTIVITY; not restarting"
        ;;
      session-dead|session_dead)
        HOOK_ASSIGNMENT=$(rig_hook_assignment "$RIG" "$PCAT_NAME")
        IFS='|' read -r HOOK_BEAD HOOK_STATUS <<< "$HOOK_ASSIGNMENT"
        if hook_restartable "$SESSION_NAME" "$HOOK_BEAD" "$HOOK_STATUS"; then
          CRASHED+=("$SESSION_NAME|$RIG|$PCAT_NAME|$HOOK_BEAD")
          log "  CRASHED: $SESSION_NAME (hook=$HOOK_BEAD)"
        fi
        ;;
      *)
        log "  SKIP $SESSION_NAME: central liveness probe inconclusive"
        ;;
    esac
  done
done <<< "$RIG_PREFIX_MAP"

log ""
log "Polecat health: ${#CRASHED[@]} crashed, ${#STUCK[@]} stuck, $HEALTHY healthy"

# --- Check deacon health -----------------------------------------------------

log ""
log "=== Deacon Health ==="

DEACON_SESSION="hq-deacon"
DEACON_ISSUE=""
DEACON_NOTICE=""

if ! tmux has-session -t "$DEACON_SESSION" 2>/dev/null; then
  log "  CRASHED: Deacon session is dead"
  DEACON_ISSUE="crashed"
else
  DEACON_HEALTH=$(gt session health "$DEACON_SESSION" --json --max-inactivity 0s 2>/dev/null \
    | jq -r '.status // empty' 2>/dev/null || true)
  case "$DEACON_HEALTH" in
    healthy|agent-hung)
      log "  OK: Deacon central health is $DEACON_HEALTH"
      ;;
    agent-dead)
      log "  ZOMBIE: Deacon agent runtime dead, session alive"
      DEACON_ISSUE="zombie"
      ;;
    session-dead)
      log "  CRASHED: Deacon central health reports session dead"
      DEACON_ISSUE="crashed"
      ;;
    *)
      log "  WARN: Deacon central liveness probe inconclusive"
      ;;
  esac

  HEARTBEAT_FILE="$TOWN_ROOT/deacon/heartbeat.json"
  if [ -z "$DEACON_ISSUE" ] && [ -f "$HEARTBEAT_FILE" ]; then
    HEARTBEAT_TIME=$(heartbeat_epoch "$HEARTBEAT_FILE" || true)
    NOW=$(date +%s)
    HEARTBEAT_AGE=$(( NOW - ${HEARTBEAT_TIME:-0} ))

    if [ "$HEARTBEAT_AGE" -gt "$DEACON_STALE_SECONDS" ]; then
      log "  NOTICE: Deacon heartbeat ${HEARTBEAT_AGE}s old (>${DEACON_STALE_SECONDS}s) — heartbeat age is notice-only; daemon owns heartbeat nudge/restart"
      DEACON_NOTICE="heartbeat_stale_${HEARTBEAT_AGE}s"
    else
      log "  OK: Deacon heartbeat ${HEARTBEAT_AGE}s old"
    fi
  fi
fi

# --- Mass death check ---------------------------------------------------------

TOTAL_ISSUES=$(( ${#CRASHED[@]} + ${#STUCK[@]} ))
MASS_DEATH=0
if [ "$TOTAL_ISSUES" -ge "$MASS_DEATH_THRESHOLD" ]; then
  log ""
  log "Mass-death candidate threshold reached ($TOTAL_ISSUES); re-checking live health before escalation"
  confirm_polecat_outages
  CRASHED=("${CONFIRMED_CRASHED[@]}")
  STUCK=("${CONFIRMED_STUCK[@]}")
  CONFIRMED_TOTAL=$(( ${#CRASHED[@]} + ${#STUCK[@]} ))

  if [ "$CONFIRMED_TOTAL" -ge "$MASS_DEATH_THRESHOLD" ]; then
    MASS_DEATH=1
    MASS_DEATH_FINGERPRINT="stuck-agent-dog:mass-death:$(mass_death_fingerprint)"
    log "MASS DEATH: $CONFIRMED_TOTAL agents down confirmed — escalating instead of restarting"
    gt escalate "Mass agent death: $CONFIRMED_TOTAL agents down" \
      -s CRITICAL \
      --source "plugin:stuck-agent-dog" \
      --fingerprint "$MASS_DEATH_FINGERPRINT" 2>/dev/null || true
  else
    log "NOTICE: mass-death candidates dropped to $CONFIRMED_TOTAL after live re-check; no CRITICAL escalation"
  fi
fi

# --- Take action --------------------------------------------------------------

if [ "$MASS_DEATH" -eq 1 ]; then
  log "Skipping per-agent restart/kill actions during mass-death escalation"
else
  # Crashed polecats: notify witness to restart
  # Note: `"${arr[@]:-}"` expands an empty array to a single empty string under
  # `set -u`, which would fire a phantom `RESTART_POLECAT: /` notification. The
  # `${arr[@]+"${arr[@]}"}` form expands to nothing when the array is empty.
  for ENTRY in ${CRASHED[@]+"${CRASHED[@]}"}; do
    IFS='|' read -r SESSION RIG PCAT HOOK <<< "$ENTRY"
    log "Requesting restart for $RIG/polecats/$PCAT (hook=$HOOK)"
    gt mail send "$RIG/witness" -s "RESTART_POLECAT: $RIG/$PCAT" --stdin <<BODY || log "  WARN: restart mail failed for $RIG/$PCAT"
Polecat $PCAT crash confirmed by stuck-agent-dog plugin.
hook_bead: $HOOK
action: restart requested
BODY
  done

  # Zombie polecats: kill zombie session, then request restart
  for ENTRY in ${STUCK[@]+"${STUCK[@]}"}; do
    IFS='|' read -r SESSION RIG PCAT HOOK REASON <<< "$ENTRY"
    log "Killing zombie session $SESSION and requesting restart"
    tmux kill-session -t "$SESSION" 2>/dev/null || true
    gt mail send "$RIG/witness" -s "RESTART_POLECAT: $RIG/$PCAT (zombie cleared)" --stdin <<BODY || log "  WARN: restart mail failed for $RIG/$PCAT"
Polecat $PCAT zombie session cleared by stuck-agent-dog plugin.
hook_bead: $HOOK
reason: $REASON
action: restart requested
BODY
  done
fi

# Deacon issues: escalate
if [ -n "$DEACON_ISSUE" ]; then
	log "Escalating deacon issue: $DEACON_ISSUE"
	DEACON_SEVERITY="HIGH"
	DEACON_FINGERPRINT="stuck-agent-dog:deacon:$DEACON_ISSUE"
	gt escalate "Deacon $DEACON_ISSUE detected by stuck-agent-dog" \
		-s "$DEACON_SEVERITY" \
		--source "plugin:stuck-agent-dog" \
		--fingerprint "$DEACON_FINGERPRINT" 2>/dev/null || true
fi

# Witness/refinery issues: read-only health escalation only. Do not restart or kill.
for ENTRY in ${CONTROL_PLANE_OUTAGES[@]+"${CONTROL_PLANE_OUTAGES[@]}"}; do
  IFS='|' read -r SESSION RIG ROLE REASON <<< "$ENTRY"
  log "Escalating control-plane issue: $RIG/$ROLE $REASON"
  gt escalate "Rig $RIG $ROLE $REASON detected by stuck-agent-dog" \
    -s CRITICAL \
    --source "plugin:stuck-agent-dog" \
    --fingerprint "stuck-agent-dog:control-plane:$SESSION:$REASON" 2>/dev/null || true
done

# --- Report -------------------------------------------------------------------

SUMMARY="Agent health: ${#CRASHED[@]} crashed, ${#STUCK[@]} stuck, $HEALTHY healthy"
[ -n "$DEACON_ISSUE" ] && SUMMARY="$SUMMARY, deacon=$DEACON_ISSUE"
[ -n "$DEACON_NOTICE" ] && SUMMARY="$SUMMARY, deacon_notice=$DEACON_NOTICE (not escalated)"
[ "${#CONTROL_PLANE_OUTAGES[@]}" -gt 0 ] && SUMMARY="$SUMMARY, control_plane_outages=${#CONTROL_PLANE_OUTAGES[@]}"
log ""
log "=== $SUMMARY ==="

gt plugin record-run --plugin stuck-agent-dog --result success \
  --title "stuck-agent-dog: $SUMMARY" --description "$SUMMARY" >/dev/null 2>&1 || true
