+++
name = "stuck-agent-dog"
description = "Context-aware stuck/crashed agent detection and restart for polecats and deacons"
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

Detects stuck or crashed polecats and deacons by inspecting tmux session context
before taking action. Unlike the daemon's blind kill-and-restart approach, this
plugin checks whether an agent is truly unresponsive before restarting.

**Design principle**: The daemon should NEVER kill workers. It detects and logs.
This plugin (running as a Dog agent with AI judgment) makes the restart decision
after inspecting tmux pane output for signs of life.

Reference: WAR-ROOM-SERIAL-KILLER.md, commit f3d47a96.

## Scope — What You May and May NOT Touch

**IN SCOPE** (these are the ONLY sessions this plugin may inspect or act on):
- Polecat sessions (`<prefix>-<name>`, e.g. `gt-minuteman`)
- Deacon session (`hq-deacon`)

**OUT OF SCOPE — NEVER touch these, under any circumstances:**
- **Crew sessions** (`<rig>-crew-<name>`, e.g. `gastown-crew-bear`). Crew lifecycle
  is managed by the overseer (human), not dogs. Crew members are persistent,
  long-lived, and user-managed. A crew session that looks idle is NOT stuck — it
  is waiting for its human. Killing a crew session destroys the overseer's active
  workspace and is a **critical incident**.
- **Mayor session** (`hq-mayor`)
- **Witness sessions** (`<rig>-witness`)
- **Refinery sessions** (`<rig>-refinery`)
- Any session not explicitly enumerated by the bash scripts in Steps 1-3

**This scope is absolute.** Do NOT extend it based on your own judgment. The bash
scripts enumerate exactly the sessions you should check. If a session does not
appear in `CRASHED[]` or `STUCK[]` arrays, it does not exist for your purposes.

## Step 1: Enumerate agents to check

Gather all polecats and the deacon session. We check both crashed sessions
(session dead, work on hook) and stuck sessions (session alive but agent hung).

```bash
echo "=== Stuck Agent Dog: Checking agent health ==="

TOWN_ROOT="$HOME/gt"
RIGS_JSON_PATH="${TOWN_ROOT}/rigs.json"

# Fallback for older/runtime-copied layouts that still expose rigs.json under mayor/.
if [ ! -f "$RIGS_JSON_PATH" ] && [ -f "$TOWN_ROOT/mayor/rigs.json" ]; then
  RIGS_JSON_PATH="$TOWN_ROOT/mayor/rigs.json"
fi

# Read rigs.json for rig names and beads prefixes
# CRITICAL: We need both the rig name (for filesystem paths like $TOWN_ROOT/$RIG/polecats/)
# and the beads prefix (for tmux session names like $PREFIX-$NAME).
# These can differ — e.g. rig "cfutons" may have prefix "CF".
if [ ! -f "$RIGS_JSON_PATH" ]; then
  echo "SKIP: rigs.json not found at $RIGS_JSON_PATH"
  exit 0
fi

if ! RIG_PREFIX_MAP=$(jq -r '
  if (.rigs | type) == "object" then
    .rigs | to_entries[] | "\(.key)|\(.value.beads.prefix // .key)"
  else
    empty
  end
' "$RIGS_JSON_PATH" 2>/dev/null); then
  echo "SKIP: could not parse rigs.json"
  exit 0
fi

# Filter out any malformed/blank rows so partial registry state fails safe.
RIG_PREFIX_MAP=$(printf '%s\n' "$RIG_PREFIX_MAP" | awk -F'|' 'NF >= 2 && $1 != "" && $2 != ""')
if [ -z "$RIG_PREFIX_MAP" ]; then
  echo "SKIP: no rigs found in rigs.json"
  exit 0
fi
```

## Step 2: Check polecat health

For each rig, enumerate polecats and check their session status.
A polecat is a concern if:
- It has hooked work (hook_bead is set)
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
        # Check hook/status through the target rig workspace before acting.
        # Only open/hooked/in_progress work is restartable.
        HOOK_BEAD=$(rig_hook_bead "$RIG" "$PCAT_NAME")
        if [ -n "$HOOK_BEAD" ] && bead_restartable "$SESSION_NAME" "$RIG" "$HOOK_BEAD"; then
          CRASHED+=("$SESSION_NAME|$RIG|$PCAT_NAME|$HOOK_BEAD")
          echo "  CRASHED: $SESSION_NAME (hook=$HOOK_BEAD)"
        fi
        ;;
      agent-dead)
        HOOK_BEAD=$(rig_hook_bead "$RIG" "$PCAT_NAME")
        if [ -n "$HOOK_BEAD" ] && bead_restartable "$SESSION_NAME" "$RIG" "$HOOK_BEAD"; then
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

The deacon session is `hq-deacon`. Check heartbeat staleness from the JSON
`timestamp` field in `deacon/heartbeat.json` (fall back to file mtime only if
the timestamp is missing or malformed). A live Deacon with no `in_progress` work
is not an actionable stuck-heartbeat event; log and skip so idle patrol backoff
does not produce escalation noise.

```bash
echo ""
echo "=== Deacon Health ==="

DEACON_SESSION="hq-deacon"
DEACON_ISSUE=""

if ! tmux has-session -t "$DEACON_SESSION" 2>/dev/null; then
  echo "  CRASHED: Deacon session is dead"
  DEACON_ISSUE="crashed"
else
  # Check deacon heartbeat file
  HEARTBEAT_FILE="$TOWN_ROOT/deacon/heartbeat.json"
  if [ -f "$HEARTBEAT_FILE" ]; then
    HEARTBEAT_TIME=$(jq -r '(.timestamp // empty) | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601? // empty' "$HEARTBEAT_FILE" 2>/dev/null)
    if [ -n "$HEARTBEAT_TIME" ]; then
      NOW=$(date +%s)
      HEARTBEAT_AGE=$(( NOW - HEARTBEAT_TIME ))

      if [ "$HEARTBEAT_AGE" -gt 1200 ]; then
        echo "  STUCK: Deacon heartbeat stale (${HEARTBEAT_AGE}s old, >20m threshold)"
        DEACON_ISSUE="stuck_heartbeat_${HEARTBEAT_AGE}s"
      else
        echo "  OK: Deacon heartbeat ${HEARTBEAT_AGE}s old"
      fi
    else
      echo "  WARN: Could not parse heartbeat timestamp from $HEARTBEAT_FILE"
    fi
  else
    echo "  WARN: No heartbeat file found at $HEARTBEAT_FILE"
  fi
fi
```

## Step 4: Inspect context before acting (AI judgment)

**This is the key difference from daemon blind-kill.** For each crashed or stuck
agent, inspect the tmux pane context to determine if restart is appropriate.

**SCOPE REMINDER: You may ONLY act on entries in the `CRASHED[]` and `STUCK[]`
arrays populated by Steps 2-3. These arrays contain ONLY polecats and deacon.
Do NOT inspect, evaluate, or act on ANY other sessions (crew, mayor, witness,
refinery). If you find yourself considering a session not in these arrays, STOP.**

**You (the dog agent) must evaluate each case:**

For CRASHED agents (session dead, work on hook):
- This is almost always a legitimate crash needing restart
- Exception: if the polecat just ran `gt done` and the hook hasn't cleared yet
- Check bead status: if the root wisp is closed, the polecat completed normally

For STUCK agents (session alive, agent dead):
- Kill the zombie session, then restart
- Exception: if pane output shows the agent is in a long-running build/test

For DEACON stuck (stale heartbeat):
- Capture pane output: `tmux capture-pane -t hq-deacon -p -S -20`
- If output shows active work (recent timestamps, command output), the heartbeat
  file may just be stale — nudge instead of kill
- If output shows no recent activity, restart is warranted
- Use a stable escalation fingerprint (`stuck-agent-dog:deacon:stuck-heartbeat`)
  for stale-heartbeat events; do not include the age seconds in the fingerprint.

**Decision framework:**
1. If central health is `session-dead` and hook status is actionable → request restart
2. If central health is `agent-dead` and hook status is actionable → clear zombie, request restart
3. If central health is `agent-hung` → observe/report only; do not restart polecat research sessions
4. If mass death detected (threshold default 3) → escalate and skip all per-agent actions

## Step 5: Mass death check

If multiple agents crashed in the same cycle, this may indicate a systemic
issue (Dolt outage, OOM, etc.). Escalate instead of blindly restarting all.
The executable script checks this before per-agent actions and skips all
restart/kill loops for that cycle.

```bash
TOTAL_ISSUES=$(( ${#CRASHED[@]} + ${#STUCK[@]} ))
if [ "$TOTAL_ISSUES" -ge "${GT_STUCK_AGENT_DOG_MASS_DEATH_THRESHOLD:-3}" ]; then
  echo "MASS DEATH: $TOTAL_ISSUES agents down in same cycle — escalating"
  gt escalate "Mass agent death: $TOTAL_ISSUES agents down" -s CRITICAL
  echo "Skipping per-agent restart/kill actions during mass-death escalation"
fi
```

## Step 6: Take action

For each agent requiring restart:

```bash
# For crashed polecats — notify witness to handle restart
for ENTRY in "${CRASHED[@]}"; do
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
for ENTRY in "${STUCK[@]}"; do
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

# For deacon issues
if [ -n "$DEACON_ISSUE" ]; then
  echo "Escalating deacon issue: $DEACON_ISSUE"
  gt escalate "Deacon $DEACON_ISSUE detected by stuck-agent-dog" \
    -s HIGH \
    --reason "Deacon issue: $DEACON_ISSUE. Context inspection completed."
fi
```

## Record Result

```bash
SUMMARY="Agent health check: ${#CRASHED[@]} crashed, ${#STUCK[@]} stuck, $HEALTHY healthy"
if [ -n "$DEACON_ISSUE" ]; then
  SUMMARY="$SUMMARY, deacon=$DEACON_ISSUE"
fi
echo "=== $SUMMARY ==="
```

On success (no issues or issues handled):
```bash
bd create "stuck-agent-dog: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:stuck-agent-dog,result:success \
  -d "$SUMMARY" --silent 2>/dev/null || true
```

On failure:
```bash
bd create "stuck-agent-dog: FAILED" -t chore --ephemeral \
  -l type:plugin-run,plugin:stuck-agent-dog,result:failure \
  -d "Agent health check failed: $ERROR" --silent 2>/dev/null || true

gt escalate "Plugin FAILED: stuck-agent-dog" \
  --severity high \
  --reason "$ERROR"
```
