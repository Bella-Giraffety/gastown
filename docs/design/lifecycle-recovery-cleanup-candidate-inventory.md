# Lifecycle And Recovery Cleanup Candidate Inventory

This file is evidence-only. It inventories witness, refinery, merge-queue, and
session-recovery cleanup candidates by comparing the current happy path with the
older witness-relay and nuke-oriented layers that still survive in code, tests,
and docs. It does not propose a combined cleanup branch.

Safety classes used here are provisional:

- `low`: local cleanup with a small semantic-equivalence or doc-only proof surface
- `medium`: reviewable in one subsystem, but needs a call-site inventory or dead-path proof
- `high`: touches lifecycle, recovery, authority routing, or cross-agent coordination
- `defer`: not cleanup-safe until a separate migration or approval path exists

Preferred PR classes map to the cleanup contracts in
`cleanup-pr-shape.md`, `cleanup-proof-standards.md`, and
`cleanup-approval-triggers.md`.

## Candidates

| Candidate | Subsystem tag | Cleanup class | Provisional safety class | Preferred PR class | Proof notes needed | Evidence |
| --- | --- | --- | --- | --- | --- | --- |
| Narrow witness-created completion relay logic so cleanup wisps and witness-driven refinery nudges only survive where crash recovery still needs them. The current happy path already pushes, creates the MR bead, nudges refinery directly, writes completion metadata for audit, and transitions the polecat to idle before witness patrol sees the result. | `lifecycle/witness-relay` | authority narrowing | high | sequenced recovery-boundary PR | Enumerate every recovery entry that still relies on `processDiscoveredCompletion` or `handlePolecatDonePendingMR`, especially interrupted `gt done`, push-failed recovery, and completion discovery after a dead session. Prove those paths remain covered before shrinking the witness relay surface for routine completions. | `internal/cmd/done.go:1180-1284`, `internal/witness/handlers.go:249-317`, `internal/witness/handlers.go:1712-1877`, `docs/design/polecat-self-managed-completion.md:96-223`, `docs/design/cleanup-history-map.md:56-85` |
| Reduce or remove merge-request cleanup wisps only after proving which states are still operationally consumed. Today witness still creates and updates cleanup wisps for pending MRs and push-failed recovery, but merged handling now tolerates the wisp being absent and persistent polecats no longer auto-nuke. | `lifecycle/cleanup-wisps` | recovery-path cleanup | high | dedicated recovery-proof PR | Inventory all live readers and writers of cleanup wisps, including merged handling, discovered completions, patrol receipts, and any operator workflows that inspect `state:merge-requested` or `state:push-failed`. Show which wisp states are still recovery-critical before deleting or collapsing them. | `internal/witness/handlers.go:249-331`, `internal/witness/handlers.go:414-466`, `internal/witness/handlers.go:529-623`, `internal/witness/handlers.go:795-817`, `internal/witness/handlers.go:1807-1869` |
| Retire the legacy mail-oriented witness/refinery protocol handler package if it is now test-only. The default protocol handlers still model merge-ready mail, witness auto-cleanup, and notification mail, while current lifecycle behavior lives in `internal/witness`, `internal/refinery`, channel events, and nudges. | `protocol/mail-lifecycle` | dead-path removal | medium | single-subsystem dead-path PR | Build a non-test call-site inventory for `WrapWitnessHandlers`, `WrapRefineryHandlers`, `DefaultWitnessHandler`, and `DefaultRefineryHandler`. Prove runtime wiring no longer routes production lifecycle through these handlers, and preserve any payload parsers still needed by active inbox-processing code. | `internal/protocol/handlers.go:63-145`, `internal/protocol/witness_handlers.go:43-144`, `internal/protocol/refinery_handlers.go:43-149`, `internal/protocol/protocol_test.go:486-739`, `internal/witness/handlers.go:118-237` |
| Remove `agent_state=done` compatibility only after a producer-consumer inventory proves no supported path still emits or depends on it. `gt done` now writes `idle` directly, but the state constant and multiple guards still special-case `done` to suppress false zombie/crash detection for older or partially migrated records. | `lifecycle/agent-state-done` | stale-compatibility removal | high | dedicated stale-compat PR | Inventory every remaining reader and writer of `AgentStateDone`, including daemon crash checks, witness zombie detection, tests, and any upgrade/migration paths for older beads. Prove that no supported agent bead, heartbeat, or session restart flow can still surface `done` before removing the compatibility branch. | `internal/beads/status.go:14-24`, `internal/cmd/done.go:1636-1667`, `internal/witness/handlers.go:1381-1415`, `internal/daemon/daemon.go:2474-2496` |
| Collapse the layered `gt done` recovery markers only with explicit recovery proof. Done-intent labels, done checkpoints, completion metadata, and heartbeat `state=exiting` overlap on purpose so witness can recover interrupted completion across push, MR creation, witness notification, and idle transition boundaries. | `recovery/done-resume-markers` | recovery/compatibility cleanup | defer | preserve-or-migrate first, cleanup second | Enumerate each failure window through `gt done` and map which layer is authoritative in that window. Prove resume still works after deleted worktrees, interrupted pushes, duplicate retries, and stale checkpoints before collapsing any marker class. If the simplification changes recovery sequencing or authority, route it through approval instead of cleanup. | `internal/cmd/done.go:364-388`, `internal/cmd/done.go:695-808`, `internal/cmd/done.go:978-1157`, `internal/cmd/done.go:1355-1479`, `internal/polecat/heartbeat.go:10-127`, `internal/witness/handlers.go:1360-1372`, `internal/witness/handlers.go:1712-1790` |
| Archive or rewrite stale witness/refinery lifecycle docs and templates that still describe witness-relay MERGE_READY forwarding, cleanup wisps as the normal merge path, or post-merge nuke semantics. Some of these are clearly historical, while others live in active template trees, so the cleanup risk is documentation authority drift rather than code behavior. | `docs/lifecycle-history` | obsolete-doc cleanup | medium | docs-only or template-only PR | Separate historical references from files that still feed runtime prompts or operator guidance. For active templates, preserve the current authority model and move witness-relay history into explicitly historical docs rather than silently deleting it. | `docs/design/polecat-lifecycle-patrol.md:97-175`, `templates/witness-CLAUDE.md:96-184`, `docs/design/architecture.md:239-257`, `docs/concepts/polecat-lifecycle.md:49-77`, `docs/design/cleanup-history-map.md:34-40` |

## Slicing Notes

- The strongest current evidence is that routine completion authority already
  sits in `gt done` plus refinery MR polling, not in witness relay code.
- The highest-risk candidates are the recovery-layer overlaps: cleanup wisps,
  `AgentStateDone`, and the done-intent/checkpoint/heartbeat stack all exist to
  preserve work across failure windows and should default to preserve-or-defer
  until the full failure-path proof is written.
- The cleanest likely early slice is the legacy `internal/protocol` handler
  package, but only if the dead-path inventory confirms it has no production
  callers beyond tests.
