# Town Bootstrap and Repair Entrypoint

> Recommendation: treat `gt up` as the native bootstrap command and `gt doctor --fix` as the native repair command.

## Problem

Gas Town already has the pieces needed to start and repair a town, but the
operator surface is split across several commands and some docs still point at
lower-level entrypoints.

That creates two kinds of confusion:

- Operators looking for the "start the town" command may reach for `gt daemon start`
  even though it only starts one subsystem.
- Operators trying to recover a broken town may not realize `gt doctor --fix`
  is the broad repair surface for redirects, settings, lifecycle defaults,
  worktrees, and database configuration.

## Existing Native Surfaces

### Bootstrap: `gt up`

`gt up` already describes itself as the idempotent boot command for Gas Town and
starts the long-lived stack needed for a healthy town:

- Dolt server
- daemon
- Deacon
- Mayor
- Witnesses
- Refineries

It is the right answer for "bring the town up" because it operates at town
scope, is safe to re-run, and has a `--restore` mode for reviving additional
sessions.

### Repair: `gt doctor --fix`

`gt doctor --fix` is already the broad repair surface. It fixes or recreates:

- stale or missing `.beads` redirects
- broken worktree metadata
- missing settings and hooks
- daemon and patrol defaults
- route and database configuration mismatches

It is the right answer for "make this town healthy again" because it performs
diagnosis before mutation and already owns the repair logic that other commands
point users toward.

## Recommendation

Standardize the operator story as:

```bash
# Fresh or stopped town
gt up

# Broken or drifted town
gt doctor --fix
gt up
```

For first-time setup, keep `gt install` as the one-time workspace creation step:

```bash
gt install ~/gt
gt up
```

## Why This Is Better Than Adding New Behavior First

The smallest correct move is to bless the entrypoints that already exist:

- No new orchestration path to maintain
- No duplicated repair logic outside `doctor`
- No ambiguity about whether a "bootstrap" command is for first install,
  service startup, or post-failure recovery

This also matches the code better than the current docs: `gt up` is town-wide
boot, while `gt daemon start` is only one component.

## Optional Follow-Up

If Gas Town still wants a more discoverable noun-verb surface, add a thin alias
instead of a new implementation path:

- `gt bootstrap` -> delegates to `gt up`
- `gt repair` -> delegates to `gt doctor --fix`

That should be a documentation and command-discoverability layer only. The
underlying behavior should continue to live in `up` and `doctor` so there is
one implementation path for boot and one for repair.

## Decision

Do not invent a new lifecycle engine for this issue.

- Official bootstrap entrypoint: `gt up`
- Official repair entrypoint: `gt doctor --fix`
- First-install entrypoint: `gt install`
- Optional future sugar: alias commands that delegate to the surfaces above
