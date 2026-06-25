+++
name = "stuck-agent-dog"
description = "Context-aware polecat restart and control-plane health escalation"
version = 1

[gate]
type = "cooldown"
duration = "5m"

[tracking]
labels = ["plugin:stuck-agent-dog", "category:health"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
severity = "high"
+++

# Stuck Agent Dog

Detects stuck or crashed polecats by inspecting central runtime health and
active hook state before taking action. It also escalates confirmed Deacon,
witness, and refinery outages without restarting or killing those control-plane
sessions.

**Design principle**: The daemon should NEVER kill workers based on blind
polecat liveness. This plugin makes polecat restart decisions from central
runtime health plus active hook state. Deacon,
witness, and refinery lifecycle is different: this plugin escalates confirmed
dead sessions/runtimes only, and never restarts or kills them. Deacon heartbeat
staleness is NOTICE-only; the Go daemon owns heartbeat nudge/restart.

Reference: WAR-ROOM-SERIAL-KILLER.md, commit f3d47a96.

## Scope — What You May and May NOT Touch

**IN SCOPE**:
- Polecat sessions (`<prefix>-<name>`, e.g. `gt-minuteman`): may be killed or
  restart-requested only when central health is dead and the hook is active.
- Deacon session (`hq-deacon`): dead session/runtime may be escalated only.
- Witness/refinery sessions (`<prefix>-witness`, `<prefix>-refinery`): read-only
  health check and escalation only; never killed or restart-requested.

**OUT OF SCOPE — NEVER touch these, under any circumstances:**
- **Crew sessions** (`<rig>-crew-<name>`, e.g. `gastown-crew-bear`). Crew lifecycle
  is managed by the overseer (human), not dogs. Crew members are persistent,
  long-lived, and user-managed. A crew session that looks idle is NOT stuck — it
  is waiting for its human. Killing a crew session destroys the overseer's active
  workspace and is a **critical incident**.
- **Mayor session** (`hq-mayor`)
- Any session not explicitly enumerated by the bash script

**This scope is absolute.** Do NOT extend it based on your own judgment. The bash
script enumerates exactly the sessions you should check.

## Step 1: Enumerate operational rigs

Use `gt rig list --json` as the authoritative rig registry. Only rigs with
`status == "operational"` are scanned. If the registry is unavailable or
unparseable, fail closed and skip the run; raw `rigs.json` does not carry enough
live dock/park state to drive CRITICAL escalations safely.

```bash
echo "=== Stuck Agent Dog: Checking agent health ==="

if ! RIG_JSON=$(gt rig list --json 2>/dev/null); then
  echo "SKIP: gt rig list --json unavailable; cannot verify operational rig state"
  exit 0
fi

RIG_PREFIX_MAP=$(printf '%s' "$RIG_JSON" | jq -r '
  if type == "array" then
    .[]
    | select(.status == "operational")
    | "\(.name)|\(.beads_prefix // .prefix // .beads.prefix // empty)"
  else
    error("expected array")
  end
' 2>/dev/null || true)

# Filter out malformed rows so partial registry state fails safe.
RIG_PREFIX_MAP=$(printf '%s\n' "$RIG_PREFIX_MAP" | awk -F'|' 'NF >= 2 && $1 != "" && $2 != ""')
if [ -z "$RIG_PREFIX_MAP" ]; then
  echo "SKIP: no operational rigs found"
  exit 0
fi
```

## Step 2: Check polecat health

For each rig, enumerate polecats and check their session status.
A polecat is a concern if:
- `gt hook show --json` reports active work with status `hooked` or `in_progress`
- Its central runtime-aware health is `session-dead` OR `agent-dead`

Polecat liveness must use `gt session health`, which wraps the central
`tmux.CheckSessionHealth` path. That path reads `GT_PROCESS_NAMES`, `GT_AGENT`,
and `GT_PANE_ID`, so opencode/node/bun detection stays in the shared runtime
configuration instead of a plugin-local process regex. Treat `agent-hung` as
observe-only for polecats; quiet OpenCode research can be legitimate live work.

```bash
CRASHED=()
STUCK=()
HEALTHY=0

while IFS='|' read -r RIG PREFIX; do
  [ -z "$RIG" ] && continue
  # List polecat directories
  POLECAT_DIR="$TOWN_ROOT/$RIG/polecats"
  [ -d "$POLECAT_DIR" ] || continue

  for PCAT_PATH in "$POLECAT_DIR"/*/; do
    [ -d "$PCAT_PATH" ] || continue
    PCAT_NAME=$(basename "$PCAT_PATH")
    # Use beads prefix (not rig name) for tmux session name
    SESSION_NAME="${PREFIX}-${PCAT_NAME}"

    HEALTH_STATUS=$(gt session health "$SESSION_NAME" --json --max-inactivity "${GT_STUCK_AGENT_DOG_MAX_INACTIVITY:-0s}" 2>/dev/null \
      | jq -r '.status // empty' 2>/dev/null || true)

    case "$HEALTH_STATUS" in
      healthy)
        HEALTHY=$((HEALTHY + 1))
        ;;
      session-dead)
        # Check active hook assignment through the target rig workspace before acting.
        HOOK_ASSIGNMENT=$(rig_hook_assignment "$RIG" "$PCAT_NAME")
        IFS='|' read -r HOOK_BEAD HOOK_STATUS <<< "$HOOK_ASSIGNMENT"
        if hook_restartable "$SESSION_NAME" "$HOOK_BEAD" "$HOOK_STATUS"; then
          CRASHED+=("$SESSION_NAME|$RIG|$PCAT_NAME|$HOOK_BEAD")
          echo "  CRASHED: $SESSION_NAME (hook=$HOOK_BEAD)"
        fi
        ;;
      agent-dead)
        HOOK_ASSIGNMENT=$(rig_hook_assignment "$RIG" "$PCAT_NAME")
        IFS='|' read -r HOOK_BEAD HOOK_STATUS <<< "$HOOK_ASSIGNMENT"
        if hook_restartable "$SESSION_NAME" "$HOOK_BEAD" "$HOOK_STATUS"; then
          STUCK+=("$SESSION_NAME|$RIG|$PCAT_NAME|$HOOK_BEAD|agent_dead")
          echo "  ZOMBIE: $SESSION_NAME (agent runtime dead, hook=$HOOK_BEAD)"
        fi
        ;;
      agent-hung)
        HEALTHY=$((HEALTHY + 1))
        echo "  OBSERVE: $SESSION_NAME runtime alive but inactive; not restarting"
        ;;
      *)
        echo "  SKIP $SESSION_NAME: central liveness probe inconclusive"
        ;;
    esac
  done
done <<< "$RIG_PREFIX_MAP"

echo ""
echo "Health summary: ${#CRASHED[@]} crashed, ${#STUCK[@]} stuck, $HEALTHY healthy"
```

## Step 3: Check deacon health

The deacon session is `hq-deacon`. A dead session or dead runtime is actionable
and escalates. Heartbeat staleness from the JSON `timestamp` field in
`deacon/heartbeat.json` (falling back to file mtime only if the timestamp is
missing or malformed) is NOTICE-only in stuck-agent-dog: it never sets
`DEACON_ISSUE` and never calls `gt escalate`. The daemon owns heartbeat
nudge/restart so the dog does not duplicate another heartbeat policy surface.

```bash
echo ""
echo "=== Deacon Health ==="

DEACON_SESSION="hq-deacon"
DEACON_ISSUE=""
DEACON_NOTICE=""

if ! tmux has-session -t "$DEACON_SESSION" 2>/dev/null; then
  echo "  CRASHED: Deacon session is dead"
  DEACON_ISSUE="crashed"
else
  DEACON_HEALTH=$(gt session health "$DEACON_SESSION" --json --max-inactivity 0s 2>/dev/null \
    | jq -r '.status // empty' 2>/dev/null || true)
  case "$DEACON_HEALTH" in
    healthy|agent-hung)
      echo "  OK: Deacon central health is $DEACON_HEALTH"
      ;;
    agent-dead)
      echo "  ZOMBIE: Deacon agent runtime dead, session alive"
      DEACON_ISSUE="zombie"
      ;;
    session-dead)
      echo "  CRASHED: Deacon central health reports session dead"
      DEACON_ISSUE="crashed"
      ;;
    *)
      echo "  WARN: Deacon central liveness probe inconclusive"
      ;;
  esac

  HEARTBEAT_FILE="$TOWN_ROOT/deacon/heartbeat.json"
  if [ -z "$DEACON_ISSUE" ] && [ -f "$HEARTBEAT_FILE" ]; then
    HEARTBEAT_TIME=$(heartbeat_epoch "$HEARTBEAT_FILE" || true)
    NOW=$(date +%s)
    HEARTBEAT_AGE=$(( NOW - ${HEARTBEAT_TIME:-0} ))

    if [ "$HEARTBEAT_AGE" -gt "${GT_STUCK_AGENT_DOG_DEACON_STALE_SECONDS:-1200}" ]; then
      echo "  NOTICE: Deacon heartbeat ${HEARTBEAT_AGE}s old (>${GT_STUCK_AGENT_DOG_DEACON_STALE_SECONDS:-1200}s) — heartbeat age is notice-only; daemon owns heartbeat nudge/restart"
      DEACON_NOTICE="heartbeat_stale_${HEARTBEAT_AGE}s"
    else
      echo "  OK: Deacon heartbeat ${HEARTBEAT_AGE}s old"
    fi
  fi
fi
```

## Step 4: Apply confirmed actions

**This is the key difference from daemon blind-kill.** For each crashed or stuck
polecat, act only after central runtime health and active hook state agree that
the polecat is currently actionable.

**SCOPE REMINDER: You may kill/restart-request ONLY entries in the `CRASHED[]`
and `STUCK[]` arrays. Those arrays contain ONLY polecats with active hook work.
Deacon, witness, and refinery may be escalated only. Do NOT inspect, evaluate,
or act on crew, mayor, or any other sessions.**

**The script evaluates each case:**

For CRASHED agents (session dead, work on hook):
- This is almost always a legitimate crash needing restart
- If `gt hook show --json` no longer reports `hooked|in_progress`, skip it.
- Do not do a separate `bd show` status lookup; the hook command is the active-work authority.

For STUCK agents (session alive, agent dead):
- Kill the zombie session, then restart
- `agent-hung` is not STUCK for polecats; central health keeps that observe-only.

For Deacon heartbeat staleness:
- Heartbeat age is NOTICE-only in this plugin, even when very stale.
- It must never set `DEACON_ISSUE` or trigger `gt escalate`.
- The daemon owns heartbeat nudge/restart; this dog only records visibility.
- Real Deacon death is handled by dead session or dead runtime checks above.

For witness/refinery outages:
- Dead session/runtime is escalated with `--source plugin:stuck-agent-dog` and a
  stable control-plane fingerprint.
- Never kill or restart witness/refinery sessions from this plugin.

**Decision framework:**
1. If central health is `session-dead` and hook status is `hooked|in_progress` → request restart
2. If central health is `agent-dead` and hook status is `hooked|in_progress` → clear zombie, request restart
3. If central health is `agent-hung` → observe/report only; do not restart polecat research sessions
4. If the Deacon session is dead or its runtime is dead → escalate.
5. If Deacon heartbeat age is stale → NOTICE-only; do not escalate.
6. If witness/refinery session or runtime is dead → escalate only.
7. If confirmed mass death remains after live re-check (threshold default 3) → escalate and skip all per-agent actions

## Step 5: Mass death check

If multiple active polecats crash in the same cycle, this may indicate a
systemic issue (Dolt outage, OOM, etc.). The executable script re-checks live
health and active hook state before CRITICAL escalation. If the confirmed count
drops below threshold, normal per-agent action proceeds for the remaining
confirmed outages.

```bash
TOTAL_ISSUES=$(( ${#CRASHED[@]} + ${#STUCK[@]} ))
MASS_DEATH=0
if [ "$TOTAL_ISSUES" -ge "$MASS_DEATH_THRESHOLD" ]; then
  confirm_polecat_outages
  CRASHED=("${CONFIRMED_CRASHED[@]}")
  STUCK=("${CONFIRMED_STUCK[@]}")
  CONFIRMED_TOTAL=$(( ${#CRASHED[@]} + ${#STUCK[@]} ))

  if [ "$CONFIRMED_TOTAL" -ge "$MASS_DEATH_THRESHOLD" ]; then
    MASS_DEATH=1
    gt escalate "Mass agent death: $CONFIRMED_TOTAL agents down" \
      -s CRITICAL \
      --source "plugin:stuck-agent-dog" \
      --fingerprint "stuck-agent-dog:mass-death"
  fi
fi
```

## Step 6: Take action

For each agent requiring restart:

```bash
if [ "$MASS_DEATH" -eq 1 ]; then
  echo "Skipping per-agent restart/kill actions during mass-death escalation"
else
# For crashed polecats — notify witness to handle restart
for ENTRY in ${CRASHED[@]+"${CRASHED[@]}"}; do
  IFS='|' read -r SESSION RIG PCAT HOOK <<< "$ENTRY"

  echo "Requesting restart for $RIG/polecats/$PCAT (hook=$HOOK)"

  gt mail send "$RIG/witness" \
    -s "RESTART_POLECAT: $RIG/$PCAT" \
    --stdin <<BODY
Polecat $PCAT crash confirmed by stuck-agent-dog plugin.
Context-aware inspection completed — agent is genuinely dead.

hook_bead: $HOOK
action: restart requested

Please restart this polecat session.
BODY

done

# For zombie polecats — kill zombie session first, then request restart
for ENTRY in ${STUCK[@]+"${STUCK[@]}"}; do
  IFS='|' read -r SESSION RIG PCAT HOOK REASON <<< "$ENTRY"

  echo "Killing zombie session $SESSION and requesting restart"
  tmux kill-session -t "$SESSION" 2>/dev/null || true

  gt mail send "$RIG/witness" \
    -s "RESTART_POLECAT: $RIG/$PCAT (zombie cleared)" \
    --stdin <<BODY
Polecat $PCAT zombie session cleared by stuck-agent-dog plugin.
Session was alive but agent process was dead.

hook_bead: $HOOK
reason: $REASON
action: restart requested

Please restart this polecat session.
BODY

done
fi

# For deacon issues
if [ -n "$DEACON_ISSUE" ]; then
  echo "Escalating deacon issue: $DEACON_ISSUE"
  DEACON_SEVERITY="HIGH"
  DEACON_FINGERPRINT="stuck-agent-dog:deacon:$DEACON_ISSUE"
  gt escalate "Deacon $DEACON_ISSUE detected by stuck-agent-dog" \
    -s "$DEACON_SEVERITY" \
    --source "plugin:stuck-agent-dog" \
    --fingerprint "$DEACON_FINGERPRINT"
fi

# For witness/refinery issues: escalate only, never kill or restart.
for ENTRY in ${CONTROL_PLANE_OUTAGES[@]+"${CONTROL_PLANE_OUTAGES[@]}"}; do
  IFS='|' read -r SESSION RIG ROLE REASON <<< "$ENTRY"
  gt escalate "Rig $RIG $ROLE $REASON detected by stuck-agent-dog" \
    -s CRITICAL \
    --source "plugin:stuck-agent-dog" \
    --fingerprint "stuck-agent-dog:control-plane:$SESSION:$REASON"
done
```

## Record Result

```bash
SUMMARY="Agent health: ${#CRASHED[@]} crashed, ${#STUCK[@]} stuck, $HEALTHY healthy"
if [ -n "$DEACON_ISSUE" ]; then
  SUMMARY="$SUMMARY, deacon=$DEACON_ISSUE"
fi
if [ -n "$DEACON_NOTICE" ]; then
  SUMMARY="$SUMMARY, deacon_notice=$DEACON_NOTICE (not escalated)"
fi
if [ "${#CONTROL_PLANE_OUTAGES[@]}" -gt 0 ]; then
  SUMMARY="$SUMMARY, control_plane_outages=${#CONTROL_PLANE_OUTAGES[@]}"
fi
echo "=== $SUMMARY ==="
```

On success (no issues or issues handled):
```bash
gt plugin record-run --plugin stuck-agent-dog --result success \
  --title "stuck-agent-dog: $SUMMARY" --description "$SUMMARY" >/dev/null 2>&1 || true
```

On failure:
```bash
gt plugin record-run --plugin stuck-agent-dog --result failure \
  --title "stuck-agent-dog: FAILED" \
  --description "Agent health check failed: $ERROR" >/dev/null 2>&1 || true

gt escalate "Plugin FAILED: stuck-agent-dog" \
  --severity high \
  --reason "$ERROR"
```
