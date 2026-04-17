# Cleanup History Map for Candidate Classification

This document maps only the historical eras that matter for cleanup review.
It is not a full project history. The goal is to answer one question fast:
which component owned cleanup-relevant state transitions when this path was
introduced?

## Scope

Track history only when it changes at least one of these:

- who decides a polecat is complete
- who routes merge-ready work to the refinery
- who owns the DONE or IDLE transition
- whether successful completion preserves or destroys the sandbox
- whether a path is happy-path cleanup, recovery, or compatibility glue

Ignore unrelated archaeology such as UI, telemetry, model, or unrelated Dolt
changes unless they directly alter one of those authority paths.

## Stop Rule

Stop digging once a candidate fits one of these buckets:

1. current happy path
2. recovery or compatibility path from an earlier authority model
3. obsolete doc-only artifact with no current code path

If classification still depends on intent instead of proof, defer the cleanup.
That is an approval-trigger case, not routine cleanup.

## Major Eras

| Era | Evidence anchor | Cleanup authority model | Cleanup implication |
|---|---|---|---|
| Early self-cleaning polecats (`v0.2.5`) | `CHANGELOG.md` (`v0.2.5`: "Self-cleaning polecat model") | Polecat completion ended in self-nuke; witness still tracked the lifecycle around completion | Code or docs that assume successful completion destroys the worker sandbox are historical |
| Witness-relay cleanup pipeline (`v0.8.0` era) | `docs/design/mail-protocol.md`, `docs/design/polecat-lifecycle-patrol.md`, `CHANGELOG.md` (`v0.8.0`: witness verifies MR bead before `MERGE_READY`) | Polecat signaled completion, witness verified and created cleanup wisps, witness sent `MERGE_READY`, refinery merged, witness handled post-merge cleanup | `POLECAT_DONE`, cleanup wisps, and witness-to-refinery relay logic originate here; they are not automatically dead just because newer docs changed |
| Persistent polecat / idle reuse (`v0.9.0`) | `CHANGELOG.md` (`v0.9.0`: persistent polecats, idle instead of nuke), `docs/design/persistent-polecat-pool.md`, `docs/concepts/polecat-lifecycle.md` | Authority for normal completion shifted away from nuke-oriented cleanup. Polecat preserves sandbox and returns to idle; refinery owns merged-branch cleanup | Any cleanup slice that assumes "complete means nuke" is crossing an era boundary and is not safe by default |
| Current hybrid self-managed completion | `docs/design/polecat-self-managed-completion.md`, `docs/design/architecture.md`, `internal/cmd/done.go`, `internal/witness/handlers.go` | Polecat owns the happy path: push, MR creation, completion metadata, direct refinery nudge, and idle transition. Witness remains as observer and crash-recovery safety net | Witness completion-discovery and cleanup-wisp logic is now recovery or compatibility code unless proven to still be on the happy path |

## Authority Handoffs That Matter

### 1. Witness stopped being the default destroyer

The important shift is not just "persistent polecats exist." It is that a
successful completion no longer implies sandbox destruction. After the
persistent-polecat work landed, `gt done` became an idle-transition and branch
sync operation instead of a self-nuke endpoint.

Cleanup consequence:

- old nuke-oriented cleanup code is suspect
- old docs that describe witness cleanup after every success are historical
- branch and sandbox cleanup must now be reviewed separately

### 2. Witness stopped being the primary happy-path relay

The witness-relay era routed routine completions through witness-owned cleanup
wisps and `MERGE_READY` forwarding. The self-managed-completion design moved the
happy path to polecat plus refinery, with witness observing for anomalies.

Cleanup consequence:

- relay code may still be needed for crash recovery
- relay code is not safe to delete just because the architecture doc says the
  witness is no longer in the critical path
- the live question is whether a path is still needed for recovery, not whether
  it is still the preferred design

### 3. Current main is a hybrid, not a clean cutover

Current code still shows both the new authority model and recovery-era residue:

- `internal/cmd/done.go` nudges the refinery directly and marks witness
  notification as observability-only
- the same command still writes completion metadata and nudges witness
- `internal/witness/handlers.go` still has completion discovery, cleanup-wisp
  creation, and refinery nudge logic, but comments frame it as a safety net

Cleanup consequence:

- classify these witness paths as recovery-first unless you can prove they are
  unreachable
- removing them requires dead-path proof or a stronger replacement proof, not a
  "new docs say otherwise" argument

## Candidate Classification Guide

Use this map when you find a cleanup candidate:

| Candidate shape | Classify it as | Default action |
|---|---|---|
| Assumes success ends in nuke or worktree destruction | pre-persistent history | Preserve or defer unless current code still relies on it |
| Uses `POLECAT_DONE`, cleanup wisps, or witness relay | witness-relay era residue | Treat as recovery or compatibility until proven dead |
| Uses direct refinery nudge, completion metadata for audit, or direct idle transition | current happy path | Cleanup can proceed if behavior is preserved |
| Only appears in docs and has no current code references | obsolete documentation | Safe to update or remove with documentation-only review |

## Practical Archaeology Boundary

For cleanup inventory work, do not dig earlier than `v0.2.5` unless the
candidate still points at a live code path from that period. The meaningful
cleanup-era boundaries are:

1. self-cleaning or self-nuking completions
2. witness-relay cleanup pipeline
3. persistent-polecat idle reuse
4. self-managed happy path with witness recovery fallback

If a candidate can be placed into one of those four buckets, archaeology is
done.
