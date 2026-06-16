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

Detects stuck or crashed polecats and deacons by inspecting tmux session context before taking action. When `run.sh` exists, dog dispatch executes that script directly; do not reinterpret or copy stale shell snippets from this document.

## Scope

Only these sessions are in scope:

- Polecat sessions named `<prefix>-<name>`.
- The deacon session `hq-deacon`.

Never inspect, kill, restart, or nudge crew, mayor, witness, refinery, or any other session type. Crew lifecycle is human-managed and out of scope.

## Liveness Model

Polecat liveness uses `gt session health <tmux-session> --json`, which wraps the central `tmux.CheckSessionHealth` path. That path reads `GT_PROCESS_NAMES` and `GT_AGENT` from the tmux session environment, then falls back to configured agent presets.

This is load-bearing: OpenCode sessions may run as `opencode`, `node`, or `bun`, and custom agents can declare their own `process_names`. Do not reintroduce Claude-only plugin-local process regexes.

Default polecat checking is process-liveness only (`GT_STUCK_AGENT_DOG_MAX_INACTIVITY=0s`) so quiet long-running research turns are not treated as stuck. Operators may set `GT_STUCK_AGENT_DOG_MAX_INACTIVITY` to a Go duration such as `30m` to report `agent-hung`; the script observes that state but does not kill a live runtime.

## Config

`run.sh` supports these environment overrides:

- `GT_STUCK_AGENT_DOG_MAX_INACTIVITY`: Go duration for optional inactivity reporting; default `0s` disables activity checks.
- `GT_STUCK_AGENT_DOG_DEACON_STALE_SECONDS`: deacon heartbeat stale threshold; default `1200`.
- `GT_STUCK_AGENT_DOG_ACTIVITY_GRACE_SECONDS`: recent tmux activity grace for stale deacon heartbeats; default matches the deacon stale threshold.
- `GT_STUCK_AGENT_DOG_MASS_DEATH_THRESHOLD`: number of crashed/stuck polecats that triggers mass-death escalation; default `3`.

## Action Policy

- `healthy`: count as healthy.
- `agent-dead`: if hooked work exists, kill only that zombie polecat session and ask the rig witness to restart it.
- `agent-hung`: runtime is alive but inactive beyond configured grace; log and observe, do not kill.
- `session-dead`: if hooked work exists and the hook bead is not closed, ask the rig witness to restart it.
- Inconclusive health probe: fail safe by skipping action for that session.
- Mass death: escalate and skip all per-agent restart/kill actions for that cycle.

Deacon checks use `deacon/heartbeat.json` age plus tmux activity cross-checking. A stale heartbeat with recent tmux activity is treated as heartbeat write divergence, not as stuck.

## Recording

On completion, `run.sh` records an ephemeral `stuck-agent-dog` chore bead with the health summary. Failures should escalate through `gt escalate` rather than killing unrelated sessions.
