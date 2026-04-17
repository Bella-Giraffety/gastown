# Cleanup Behavior Invariants for Scoped Gastown Subsystems

This document defines what counts as behavior for cleanup-only work in the
current gastown cleanup program. The goal is to stop arguing from reviewer
intuition and instead require every cleanup slice to preserve an explicit
contract.

It pairs with
[`cleanup-proof-standards.md`](cleanup-proof-standards.md),
[`cleanup-approval-triggers.md`](cleanup-approval-triggers.md), and
[`cleanup-pr-shape.md`](cleanup-pr-shape.md).

## Core Rule

For the scoped cleanup program, behavior means more than returned values. A
cleanup-only change must preserve all of the following inside its claimed
subsystem boundary unless the change is explicitly routed through the approval
path:

- externally visible outputs and prompts
- errors, exit status, and fallback selection
- side effects on git state, files, env-derived config, mail, nudges, and beads
- operator-visible state transitions and logs
- subsystem-boundary contracts about who decides, routes, records, or recovers

If a proposed cleanup changes any of those on purpose, or cannot prove they are
unchanged, it is not cleanup-only.

## Invariant Classes

Every cleanup slice in this program should name which invariant class it is
preserving.

| Class | What must stay the same | Typical proof forms |
| --- | --- | --- |
| Surface invariant | User-facing text, prompts, logs, exit behavior, file locations, env precedence | characterization tests, operational evidence |
| Side-effect invariant | Git mutations, bead writes, mail/nudge emission, hook installation, session env writes | line-level equivalence, call-site inventory, operational evidence |
| Ordering invariant | Startup order, fallback order, lifecycle sequencing, recovery sequencing | characterization tests, operational evidence, semantic equivalence |
| Authority invariant | Which component owns identity, routing, completion, recovery, or cleanup decisions | call-site inventory, dead-path proof, design/code citation |
| Boundary invariant | Which callers and adjacent subsystems rely on the path | call-site inventory, existing-test rationale, dead-path proof |

## Scoped Subsystem Invariants

### 1. Startup and Bootstrap

Cleanup-only work must preserve these startup/bootstrap behaviors:

- Startup beacons keep the same recipient/sender/topic semantics and the same
  assignment instruction contract: assigned agents are told to run
  `gt prime --hook` before acting on hooked work.
- The startup capability matrix stays intact: hook-capable runtimes get context
  from hooks, non-hook runtimes are told to run `gt prime`, and delayed nudges
  still wait long enough for prime to complete.
- Autonomous-role startup still couples prime with mail-check injection only in
  the cases the current runtime matrix already does.
- Polecat bootstrap still starts work from the rig's canonical remote base and
  preserves the current worktree layout compatibility rules.
- Startup cleanup may remove duplication, but it must not change who receives
  startup instructions, when work instructions are delivered, or which branch
  and worktree shape a recovered session starts from.

Not cleanup-safe by default:

- changing beacon wording in a way that alters startup instructions or recipient
  interpretation
- changing the order of prime, prompt delivery, work nudges, or mail injection
- removing the old worktree-layout compatibility path without proving universal
  migration
- changing which remote/default branch establishes the canonical session base

Primary anchors:

- `internal/session/startup.go`
- `internal/runtime/runtime.go`
- `internal/polecat/session_manager.go`
- `docs/guides/local-rig-bootstrap.md`

### 2. Routing and Identity

Cleanup-only work must preserve these routing/identity behaviors:

- Agent identity remains the same slash-path contract for `BD_ACTOR`,
  `GIT_AUTHOR_NAME`, mail addressing, and audit/event attribution.
- Routing precedence stays the same when resolving rig, session, and beads
  context from env vars, worktree location, and routing tables.
- Cleanup does not change which component is authoritative for mapping issue
  prefixes to rig databases or role identities to runtime env.
- Cleanup does not change the meaning of identity-bearing metadata written to
  git, beads, telemetry, or startup beacons.

Not cleanup-safe by default:

- renaming identity formats, recipient formats, or attribution fields
- changing env-over-cwd precedence, route-prefix interpretation, or fallback
  resolution without an explicit migration
- moving authority for identity derivation or route ownership between subsystems

Primary anchors:

- `docs/concepts/identity.md`
- `docs/design/architecture.md`
- `internal/config/agents.go`
- `internal/cmd/prime.go`
- `internal/cmd/done.go`

### 3. Lifecycle and Recovery

Cleanup-only work must preserve these lifecycle/recovery behaviors:

- The happy path for `gt done` keeps the same externally relevant outcome:
  validate state, submit merge-ready work, record completion metadata, clear the
  hook, sync the sandbox, and transition the polecat to idle rather than silently
  changing to a different completion model.
- Exit-status meaning stays stable: `COMPLETED`, `ESCALATED`, and `DEFERRED`
  keep their current operator-visible semantics.
- Witness and refinery remain on the same authority boundaries for observation,
  recovery, and merge processing unless a change is explicitly treated as design
  work.
- Recovery paths, zombie detection, stale-session handling, and compatibility
  residue are preserved unless dead-path proof covers all supported callers and
  failure modes.
- Cleanup does not change when branches are pushed, when hooks are cleared, when
  sandboxes are preserved, or when recovery escalates instead of destroying work.

Not cleanup-safe by default:

- altering `gt done` state transitions, cleanup timing, or branch-sync behavior
- collapsing witness recovery logic just because the happy path moved elsewhere
- deleting compatibility or recovery paths based only on preferred architecture
  docs instead of reachability proof
- changing MR/refinery notification semantics, completion metadata meaning, or
  idle-vs-nuke outcomes

Primary anchors:

- `docs/concepts/polecat-lifecycle.md`
- `docs/design/polecat-self-managed-completion.md`
- `docs/design/cleanup-history-map.md`
- `internal/cmd/done.go`
- `internal/witness/handlers.go`

### 4. Hooks and Runtime Integration

Cleanup-only work must preserve these hook/runtime behaviors:

- Provider-specific settings and hook files continue to install into the same
  managed locations with the same role-aware template selection rules.
- Existing compatibility guarantees stay intact until migration is proven:
  stale hook upgrade handling, `CLAUDE_SESSION_ID` fallback, old worktree layout
  readers, and `GT_HOME` config lookup precedence are behavior, not clutter.
- Runtime session ID lookup, hook-vs-prompt fallback, and startup prompt delay
  rules keep the same observable results for supported runtimes.
- Cleanup does not change whether slash-command provisioning, hook sync, or
  runtime initialization writes into a shared settings dir versus a worktree dir.

Not cleanup-safe by default:

- changing hook file locations, template-selection precedence, or sync ownership
- removing env/config fallback paths without a full producer-consumer inventory
- normalizing runtime differences that currently affect prompts, startup timing,
  or session resumption
- changing `GT_HOME` read precedence or hook config search order as part of a
  cleanup-only PR

Primary anchors:

- `docs/HOOKS.md`
- `docs/design/hook-runtime-cleanup-candidate-inventory.md`
- `internal/runtime/runtime.go`
- `internal/hooks/installer.go`
- `internal/hooks/config.go`

## What Counts as Proof Against This Contract

An acceptable cleanup proof names the preserved invariant explicitly. Good
examples:

- "Startup ordering invariant preserved: beacon, prime, delayed work nudge, and
  mail injection remain in the same order for non-hook runtimes."
- "Identity authority invariant preserved: `GT_RIG` still wins over cwd-derived
  rig detection, and `BD_ACTOR` format is unchanged."
- "Lifecycle authority invariant preserved: witness remains recovery-only, while
  `gt done` keeps the same idle transition and completion metadata writes."
- "Runtime compatibility invariant preserved: `GT_HOME` lookup order and
  `CLAUDE_SESSION_ID` fallback are untouched by this duplicate-collapse change."

Insufficient cleanup claims:

- "This only removes indirection."
- "The tests still pass."
- "The new path is clearer."
- "Operators should not care about this fallback."

## Cleanup vs Approval-Gated Change

Use this decision rule before calling a PR cleanup-only:

1. Name the touched invariant class and scoped subsystem.
2. State which outputs, errors, side effects, ordering, authority path, and
   boundary contract are preserved.
3. Cite the proof form and evidence bundle.
4. If any preserved item is actually changing, route the work through
   `cleanup-approval-triggers.md` instead of calling it cleanup.

That distinction is the contract: cleanup-only work preserves these invariants;
approval-gated work intentionally changes one of them.
