# Native Rig Convergence And Repair Ownership

**Issue:** gs-68t.1
**Status:** Proposal
**Author:** gastown/polecats/guzzle

## Problem

Rig repair logic currently exists in several places that each own only part of the
truth:

- `internal/rig/manager.go` creates rigs, verifies `metadata.json`, and attempts
  local repair during `gt rig add`.
- `internal/daemon/daemon.go` calls `doltserver.EnsureAllMetadata()` at daemon
  startup to repair every rig opportunistically.
- `internal/doctor/rig_config_sync_check.go` and
  `internal/doctor/stale_dolt_port_check.go` diagnose and fix related drift.
- `internal/cmd/repair.go` is a thin wrapper over a subset of doctor checks.
- `internal/polecat/manager.go` repairs polecat worktrees, resets agent beads,
  and re-establishes shared beads state when a sandbox is stale.

This works as a collection of patches, but ownership is blurred:

- no single component is responsible for converging a rig to canonical state
- repair can happen at rig-add time, daemon-start time, doctor time, or when a
  polecat is already broken
- the system mixes *diagnosis*, *convergence*, and *role-specific recovery*
- user guidance still centers on `gt doctor --fix`, even when the rig could
  repair itself natively

The `PROJECT IDENTITY MISMATCH` failure mode is the clearest symptom. A rig can
exist, have a running Dolt server, and still be unusable because its metadata,
port config, registry state, and local worktree state drifted out of alignment.

## Current Ownership Model

Today the effective ownership looks like this:

| Concern | Current owner | Notes |
|---------|---------------|-------|
| Rig metadata correctness | `doltserver.EnsureMetadata()` | Used from multiple callers |
| Cross-rig metadata sweep | daemon startup | Best-effort, not explicit rig lifecycle |
| Detecting config drift | doctor checks | User-invoked or patrol-invoked |
| Repair command UX | `gt repair` | Only covers some checks |
| Polecat sandbox recovery | polecat manager | Handles worktree-level repair, not rig convergence |

This splits authority horizontally. The code knows how to repair many pieces,
but no layer owns the invariant: "a rig converges itself to a valid operational
shape before work is dispatched into it."

## Proposal

Make **the rig** the unit of convergence and make **rig convergence** the owner
of repairable infrastructure drift.

The model:

1. Define a canonical rig state from repo config, rig registry, and authoritative
   Dolt server config.
2. Provide one convergence entry point that computes drift and applies all safe,
   deterministic repairs for that rig.
3. Have callers ask for convergence instead of individually calling repair helpers.
4. Keep role-specific recovery separate: polecat worktree repair remains a
   sandbox concern, not a rig identity/config concern.

In practice, this means introducing a first-class rig convergence service that
absorbs the safe parts of current doctor/repair logic.

## Canonical Rig State

For each rig, convergence should enforce these invariants:

- rig registry entry exists and matches the rig directory name
- `config.json` exists and agrees with registry-managed prefix and URLs
- `mayor/rig/.beads/config.yaml` has the correct prefix configuration
- `mayor/rig/.beads/metadata.json` points at the canonical Dolt database for the rig
- `metadata.json` server host and port match authoritative Dolt config
- the Dolt database exists
- the rig identity bead exists
- polecat/shared beads setup can rely on the above being true

This folds the existing checks into one contract instead of scattering them
across commands.

## Ownership Boundaries

The ownership split should become:

| Domain | Owner | Responsibility |
|--------|-------|----------------|
| Rig identity/config convergence | rig convergence service | Canonicalize metadata, config, database, and rig identity bead |
| Town-wide scheduling of convergence | daemon / doctor / `gt repair` | Decide *when* to invoke convergence |
| Polecat sandbox repair | polecat manager | Rebuild stale worktrees and agent bead state |
| Human-facing diagnostics | doctor | Explain drift, surface non-safe repairs, report remaining issues |

This gives `gt repair` a clear job: invoke rig convergence intentionally.
It gives doctor a clear job: diagnose and optionally trigger convergence. It
keeps polecat repair from growing into another generic infrastructure fixer.

## Convergence Flow

Recommended shape:

```text
caller
  -> rig.Converge(rigName)
       -> load canonical rig inputs
       -> compute drift
       -> apply deterministic fixes in dependency order
       -> return report { changed, warnings, unresolved }
```

Suggested fix order:

1. registry/config consistency
2. beads config prefix consistency
3. metadata host/port/database correction
4. Dolt database existence
5. rig identity bead creation

That order matters because later repairs depend on earlier naming decisions.

## Command Implications

This proposal does not require a large UX change, but it clarifies behavior:

- `gt rig add` should call rig convergence for the new rig, not ad hoc identity repair.
- daemon startup should converge each rig before patrol activity begins.
- `gt repair` should become the explicit user-facing wrapper for rig convergence.
- `gt doctor --fix` should delegate safe rig fixes to the same convergence engine,
  then report anything still unresolved.

One engine, multiple callers.

## Why This Is Better

- Removes duplicate repair paths that can drift apart.
- Makes ownership legible: rig drift belongs to rig convergence.
- Lets daemon and doctor share logic instead of duplicating fix rules.
- Catches identity mismatch before a polecat hits `bd show` and stalls on hook.
- Keeps polecat repair focused on sandbox recovery, where it already has strong logic.

## Non-Goals

- Replacing doctor as the main diagnostic surface.
- Moving worktree repair into the rig layer.
- Making all repairs automatic; only deterministic, low-risk repairs should converge.
- Solving cross-project Dolt misrouting by guesswork when the authoritative town
  config is itself ambiguous.

## Implementation Sketch

### Phase 1: Extract convergence engine

- Introduce a rig-scoped convergence type, likely under `internal/rig`.
- Move safe fix logic out of doctor checks into reusable functions.
- Make doctor checks read-only wrappers around the same invariant definitions.

### Phase 2: Rewire callers

- `gt rig add` -> converge new rig
- daemon startup -> converge all rigs
- `gt repair` -> call converge explicitly
- `gt doctor --fix` -> call converge, then print remaining findings

### Phase 3: Tighten failure surfaces

- fail fast on hook dispatch when rig convergence reports unresolved identity drift
- improve the error message to point at the rig convergence owner, not a vague
  collection of fallback commands

## Open Questions

- Should town-level convergence be a separate wrapper around per-rig convergence,
  or just a loop in daemon/doctor?
- Should convergence emit structured events so Witness can report chronic drift?
- Should `gt prime --hook` run a lightweight converge check before instructing a
  polecat to execute, or is daemon-start convergence sufficient?

## References

- `internal/rig/manager.go`
- `internal/doltserver/doltserver.go`
- `internal/daemon/daemon.go`
- `internal/doctor/rig_config_sync_check.go`
- `internal/doctor/stale_dolt_port_check.go`
- `internal/cmd/repair.go`
- `internal/polecat/manager.go`
