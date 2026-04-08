# Runtime Town Root Ownership

> **Bead:** gs-eir.3
> **Date:** 2026-04-08
> **Author:** nitro (gastown polecat)
> **Status:** Proposal

---

## Problem

Gas Town currently treats the canonical town root as both:

1. A long-lived ambient shell hint (`GT_TOWN_ROOT`, `GT_RIG`)
2. A general runtime identity/env variable (`GT_ROOT`)

That makes the same concept visible at too many scopes. In this session, the
result was a real misroute:

- Actual town root: `/home/coder/gt`
- Inherited `GT_ROOT`: `/home/coder/coder-dotfiles`
- Inherited `GT_TOWN_ROOT`: `/home/coder/coder-dotfiles`
- `bd show gs-eir.3` failed with a project identity mismatch

The failure is not that town-root discovery exists in multiple places. The
failure is that ownership is unclear, so stale values can outrank local truth.

## Proposal

Define a single ownership model for the canonical town root:

1. `GT_TOWN_ROOT` is the canonical runtime town-root variable.
2. `GT_TOWN_ROOT` may be set only by Gas Town-controlled runtime boundaries:
   `gt prime`, session manager startup, tmux global env repair, and shell rig
   detection when the user is actively inside a Gas Town repo.
3. `GT_ROOT` is demoted to a compatibility alias, not an independently owned
   source of truth.
4. Any code that reads `GT_ROOT` should interpret it as: "fallback only when
   `GT_TOWN_ROOT` is absent and the value validates as a workspace."

This keeps one canonical value while avoiding a flag day.

## Ownership Boundaries

### 1. Interactive shell scope

Owned variable: `GT_TOWN_ROOT`

- Shell integration may export `GT_TOWN_ROOT` and `GT_RIG`.
- Shell integration should stop exporting `GT_ROOT`.
- Leaving a Gas Town repo should unset `GT_TOWN_ROOT` and `GT_RIG`.

Reason: shell hooks model current location, not process identity.

### 2. Agent session scope

Owned variable: `GT_TOWN_ROOT`

- Polecat/crew/mayor/witness/refinery startup must inject `GT_TOWN_ROOT`.
- `gt prime` session repair should treat `GT_TOWN_ROOT` as core identity and
  repair it the same way it already repairs `GT_ROOT`.
- Session-local commands may also receive `GT_ROOT`, but only as a derived copy
  from `GT_TOWN_ROOT` for older call sites.

Reason: agent sessions need a stable canonical value even after cwd changes or
worktree cleanup.

### 3. Subprocess scope

Owned variables: none by inheritance

- Subprocesses that need a town root should receive it explicitly.
- Helpers that intentionally isolate env (`CleanGTEnv`, isolated `bd`/`gt`
  commands, convoy helpers) should preserve Dolt connection info but strip
  inherited town-root identity unless the caller re-adds it deliberately.
- `BEADS_DIR` remains opt-in and command-local because global inheritance breaks
  routing.

Reason: subprocess correctness should come from explicit injection, not ambient
global state.

## Minimal Code Direction

The smallest safe change set is:

1. Make `GT_TOWN_ROOT` the variable repaired by `gt prime` alongside session
   identity.
2. Stop shell `gt rig detect` from exporting `GT_ROOT`; keep `GT_TOWN_ROOT` and
   `GT_RIG` only.
3. Keep `GT_ROOT` injection inside agent startup/config generation for now, but
   always derive it from `GT_TOWN_ROOT`.
4. Audit fallback readers so precedence is:
   workspace discovery -> `GT_TOWN_ROOT` -> `GT_ROOT`.
5. Add one doctor check or env invariant test that fails when `GT_ROOT` and
   `GT_TOWN_ROOT` disagree inside an agent session.

This preserves compatibility while establishing one owner.

## Why This Is Minimal

- No rename across the codebase
- No new environment variable
- No behavioral change for callers that still need `GT_ROOT`
- Fixes the highest-risk failure mode: stale inherited root outranking the real
  active town

## Expected Invariants

After this change:

1. In an agent session, `GT_TOWN_ROOT` is always present and valid.
2. If `GT_ROOT` is present, it equals `GT_TOWN_ROOT`.
3. A stale parent-shell `GT_ROOT` cannot redirect `bd` or `gt` away from the
   active town.
4. Shell context and agent identity stop competing for ownership of the same
   concept.

## Follow-On Work

If the minimal fix lands and remains stable, a later cleanup can remove most
runtime reads of `GT_ROOT` entirely and reserve it for legacy compatibility.
