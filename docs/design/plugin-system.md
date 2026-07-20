# Plugin System Design

> **Status: Implemented runtime contract**
>
> Design document for the Gas Town plugin system.
> Written 2026-01-11, crew/george session.

## Problem Statement

Gas Town needs extensible, project-specific automation that runs during Deacon patrol cycles. The immediate use case is rebuilding stale binaries (gt, bd, wv), but the pattern generalizes to any periodic maintenance task.

Current state:
- `~/gt/plugins/` and `<rig>/plugins/` are scanned by the daemon
- Plugin outcomes are recorded as ledger receipts
- `run.sh` plugins execute in Go before any AI dog step
- Dog dispatch is used only for agent plugins or script exit-code 10 handoff

## Design Principles Applied

### Discover, Don't Track
> Reality is truth. State is derived.

Plugin state (last run, run count, results) lives on the ledger as wisps, not in shadow state files. Gate evaluation queries the ledger directly.

### Deterministic Work Before AI
> Code guards run in code.

If a plugin ships `run.sh`, the daemon/CLI executes that script directly before any AI dog step. The dog receives `plugin.md` instructions only when no script exists or when `run.sh` exits with code 10 to request an AI follow-up.

### MEOW Stack Integration

| Layer | Plugin Analog |
|-------|---------------|
| **M**olecule | `plugin.md` - work template with TOML frontmatter |
| **E**phemeral | Plugin-run wisps - high-volume, digestible |
| **O**bservable | Plugin runs appear in `bd activity` feed |
| **W**orkflow | Gate → Dispatch → Execute → Record → Digest |

---

## Architecture

### Plugin Locations

```
~/gt/
├── plugins/                      # Town-level plugins (universal)
│   └── README.md
├── gastown/
│   └── plugins/                  # Rig-level plugins
│       └── rebuild-gt/
│           └── plugin.md
├── beads/
│   └── plugins/
│       └── rebuild-bd/
│           └── plugin.md
└── wyvern/
    └── plugins/
        └── rebuild-wv/
            └── plugin.md
```

**Town-level** (`~/gt/plugins/`): Universal plugins that apply everywhere.
**Rig-level** (`<rig>/plugins/`): Project-specific plugins.

The Deacon scans both locations during patrol.

### Execution Model: Shared Runtime

**Key insight**: plugin execution, dog dispatch, receipts, cooldown, and retry use one runtime contract.

Script plugins:

```
daemon/CLI runtime
├─ execute plugins/<name>/run.sh in the plugin directory
├─ enforce [execution].timeout or the 5m default
├─ use a minimal allowlisted environment
├─ retain only bounded stdout/stderr output
└─ record the authoritative outcome
```

Exit-code contract:
- `0`: script completed the plugin run; record success; do not dispatch a dog.
- `10`: script completed deterministic pre-work and requests an AI dog step; dispatch a dog with `plugin.md` instructions and the bounded script output tail. The dog must not rerun `run.sh`.
- Any other exit, execution error, or timeout: record a retryable failure; do not dispatch a dog.

Agent-only plugins have no `run.sh`; the runtime dispatches them directly to a dog.

### State Tracking: Wisps on the Ledger

Each runtime outcome creates a plugin-run receipt through `gt plugin record-run`/`internal/plugin.Recorder`:

```bash
gt plugin record-run --plugin rebuild-gt --result success --rig gastown \
  --title "Plugin: rebuild-gt [success]" \
  --description "Rebuilt gt: abc123 → def456 (5 commits)"
```

Cooldown gate evaluation queries receipts instead of state files. Runtime-owned receipts explicitly mark `cooldown:counted` when they close a gate; `gt plugin record-run` also adds explicit cooldown/retry labels for manual or dog-completion receipts. Legacy persisted `success`/`skipped` receipts without authority/cooldown labels still count so old history keeps working. Retryable failures, timeouts, no-dog, and dispatch failures do not hide retry behind cooldown.

```bash
# Cooldown check: counted outcomes in last hour
bd list --all --label type:plugin-run --label plugin:rebuild-gt --label cooldown:counted --created-after <timestamp>
```

**Derived state** (no state.json needed):

| Query | Command |
|-------|---------|
| Last run time | `gt plugin history X --limit 1 --json` |
| Run count | `gt plugin history X --json \| jq length` |
| Last result | Parse `result:` label from latest wisp |
| Failure rate | Count `result:failure` vs total |

### Digest Pattern

Like cost digests, plugin wisps accumulate and get squashed daily:

```bash
gt plugin digest --yesterday
```

Creates: `Plugin Digest 2026-01-10` bead with summary
Deletes: Individual plugin-run wisps from that day

This keeps the ledger clean while preserving audit history.

---

## Plugin Format Specification

### File Structure

```
rebuild-gt/
├── plugin.md      # Definition with TOML frontmatter
└── run.sh         # Optional deterministic executable
```

### plugin.md Format

```markdown
+++
name = "rebuild-gt"
description = "Rebuild stale gt binary from source"
version = 1

[gate]
type = "cooldown"
duration = "1h"

[tracking]
labels = ["plugin:rebuild-gt", "rig:gastown", "category:maintenance"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
+++

# Rebuild gt Binary

Instructions for the dog worker to execute...
```

### TOML Frontmatter Schema

```toml
# Required
name = "string"           # Unique plugin identifier
description = "string"    # Human-readable description
version = 1               # Schema version (for future evolution)

[gate]
type = "cooldown|cron|condition|event|manual"
# Type-specific fields:
duration = "1h"           # For cooldown
schedule = "0 9 * * *"    # For cron
check = "gt stale -q"     # For condition (exit 0 = run)
on = "startup"            # For event

[tracking]
labels = ["label:value", ...]  # Labels for execution wisps
digest = true|false            # Include in daily digest

[execution]
timeout = "5m"            # Max execution time
notify_on_failure = true  # Escalate on failure
severity = "low"          # Escalation severity if failed
```

### Gate Types

| Type | Config | Behavior |
|------|--------|----------|
| `cooldown` | `duration = "1h"` | Query wisps, run if none in window |
| `cron` | `schedule = "0 9 * * *"` | Run on cron schedule |
| `condition` | `check = "cmd"` | Run check command, run if exit 0 |
| `event` | `on = "startup"` | Run on Deacon startup |
| `manual` | (no gate section) | Never auto-run, dispatch explicitly |

### Instructions Section

The markdown body after the frontmatter contains agent-executable instructions. For script plugins, those instructions are only used after `run.sh` exits 10. The runtime executes `run.sh`; dogs must not run it from mail.

Standard sections:
- **Detection**: Check if action is needed
- **Action**: The actual work
- **Record Result**: Optional script-local notes; the runtime owns the authoritative receipt while `GT_PLUGIN_RUNNER_ACTIVE=1`
- **Notification**: On success/failure

---

## New Commands Required

- **`gt stale`** -- Expose binary staleness check (human-readable, `--json`, `--quiet` exit code)
- **`gt dog dispatch --plugin <name>`** -- Run the shared plugin runtime and dispatch an AI dog only when required
- **`gt plugin list|show|run|digest|history`** -- Plugin management and execution history

---

## Implementation Status

- Plugin scanning, cooldown gates, history receipts, and CLI listing/show/history are implemented.
- The shared runtime owns script execution, dog dispatch, cooldown-counted receipts, retryable failure receipts, timeout, output bounds, and runner environment.
- `gt plugin run`, `gt dog dispatch --plugin`, and daemon auto-dispatch all use the same runtime contract.
- Cron, condition, event gates, and digest squashing remain future work unless a plugin explicitly implements that behavior itself.

---

## Open Questions

1. **Plugin discovery in multiple clones**: If gastown has crew/george, crew/max, crew/joe - which clone's plugins/ dir is canonical? Probably: scan all, dedupe by name, prefer rig-root if exists.

2. **Dog assignment**: Should specific plugins prefer specific dogs? Or any idle dog?

3. **Plugin dependencies**: Can plugins depend on other plugins? Probably not in v1.

4. **Plugin disable/enable**: How to temporarily disable a plugin without deleting it? Label on a plugin bead? `enabled = false` in frontmatter?

---

## References

- PRIMING.md - Core design principles
- mol-deacon-patrol.formula.toml - Patrol step plugin-run
- ~/gt/plugins/README.md - Current plugin stub
