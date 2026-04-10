# Upstream Drift Research - 2026-04-10

This note captures the upstream and runtime drift relevant to `hq-1jv.1`.

## Summary

- The hook's two branch vars are not currently diverged: `upstream-rebuild-main` and `integration/repair-forked-gastown-startup-runtime-and-routing` both resolve to `5c977636` in this worktree.
- The meaningful code drift is `origin/main` versus `origin/upstream-rebuild-main`: `6677` commits on the `upstream-rebuild-main` side and `18` commits on the `main` side, with `345 files changed`, `17668 insertions`, and `2564 deletions`.
- The live blocker is not a missing fix in source alone. `gt prime --hook` and `bd show hq-1jv.1` still fail with `PROJECT IDENTITY MISMATCH` even though this branch already contains routing and Dolt-env hardening for that failure class.

## Branch Topology

Commands run from `gastown/polecats/rust/gastown`:

```bash
git log --oneline --decorate --graph --max-count=40 upstream-rebuild-main integration/repair-forked-gastown-startup-runtime-and-routing polecat/rust-mnthxju5
git rev-list --left-right --count origin/main...origin/upstream-rebuild-main
git diff --shortstat origin/main..origin/upstream-rebuild-main
```

Findings:

- `upstream-rebuild-main`, the integration branch, and `HEAD` all point at `5c977636`.
- `origin/main...origin/upstream-rebuild-main` reports `6677  18`.
- `origin/main..origin/upstream-rebuild-main` reports `345 files changed, 17668 insertions(+), 2564 deletions(-)`.

Recent commits only on `origin/upstream-rebuild-main` are concentrated on startup, routing, and Dolt scoping:

- `c54c7e95` `fix: reject mismatched Dolt server database sets`
- `0a34d6b8` `fix(startup): port promptless startup and pane-target nudges`
- `81f7a371` `fix: tighten cross-rig mail and bd routing guards`
- `7f97a425` `fix: derive polecat dolt env from town root`
- `81d40ac7` `fix: keep canonical rig DBs and cross-rig bd scope aligned`

Recent commits only on `origin/main` are also targeting routing/runtime correctness:

- `b6fe9e0a` `fix(deps): bump beads v0.63.3 -> v1.0.0`
- `69373602` `fix(beads): route cross-rig agent bead creation from town root`
- `5a26a659` `fix: preserve town-root bead routing`
- `57b78623` `fix: force local dolt sql helper to use TCP client mode`
- `1ee320d9` `fix: harden reaper DB selection and opencode wrapper startup`

The practical result is that both lines appear to hold partial fixes for the same class of failures, but they are not yet reconciled.

## Upstream Release State

Upstream GitHub metadata:

```bash
gh repo view steveyegge/gastown
gh repo view steveyegge/beads
gh release list -R steveyegge/gastown --limit 20
gh release list -R steveyegge/beads --limit 20
git ls-remote https://github.com/steveyegge/gastown.git refs/heads/main refs/tags/v1.0.0
git ls-remote https://github.com/steveyegge/beads.git refs/heads/main refs/tags/v1.0.0
```

Findings:

- `steveyegge/gastown` latest release: `v1.0.0` on `2026-04-03T05:46:01Z`.
- `steveyegge/beads` latest release: `v1.0.0` on `2026-04-03T05:38:27Z`.
- Upstream `gastown` main now points at `9f962c4`, while tag `v1.0.0` points at `1165b094`.
- Upstream `beads` main now points at `66d7df9`, while tag `v1.0.0` points at `d6fdab0`.

This repo also identifies itself as `1.0.0`:

- `CHANGELOG.md:10` starts the `1.0.0` release section.
- `npm-package/package.json:3` sets package version `1.0.0`.

That means the current problem is not simply "still pre-1.0". It is post-`1.0.0` fork/runtime drift, plus unreconciled branch repair work.

## Live Runtime Drift

Commands run:

```bash
gt prime --hook
gt hook
gt dolt status
bd show hq-1jv.1
gt version --verbose
bd version
dolt version
```

Findings:

- `gt prime --hook` identifies the correct hooked bead, but `bd show hq-1jv.1` fails with `PROJECT IDENTITY MISMATCH`.
- The mismatch is between the town project ID `5a98cfd9-470c-4b0b-98c0-d7920ab5a539` and the gastown project ID `b0ca637b-e51f-4e5b-b83f-eb4d3b60a38e`.
- `gt dolt status` shows the server is healthy and serving both `gastown` and `hq`, so the failure is path/database selection, not server availability.
- Installed `gt` is a dirty build at `f9bac5a` built with `go1.25.8`, while this worktree is at `5c977636`.
- Installed `bd` reports `1.0.0 (dev)`.
- Installed `dolt` reports `1.84.0`, and warns that `1.86.0` is newer.

This is important because it means a user can be running a binary that does not correspond to the checked-out source being inspected or edited.

## Source Evidence

The current source tree already includes targeted protections for this area:

- `internal/beads/routes.go:345-359` routes a bead by prefix and sends `hq-` work to town root through `ResolveHookDir`.
- `internal/polecat/session_manager.go:778-811` strips stale `GT_*`, `BEADS_DIR`, `BEADS_DB`, and `BEADS_DOLT_SERVER_DATABASE` values before invoking `bd`.
- `internal/polecat/session_manager_test.go:303-419` tests both `hq-` town-root routing and stale Dolt-env stripping.
- `internal/cmd/prime_session.go:321-336` resolves the agent bead and hook bead through `ResolveHookDir` during autonomous startup detection.
- `internal/cmd/prime.go:180-190` explicitly warns agents not to interpret hook-query database errors as "no work assigned".
- `internal/doltserver/doltserver.go:3071-3089` repairs wrong `dolt_database` values in `metadata.json` because they cause identity mismatches.

The source intent is correct. The live behavior shows some hook/control-plane path is still resolving the HQ lookup under gastown identity, or the installed binary is bypassing fixes present in the checked-out source.

## Assessment For 15-Agent Fan-Out

Upstream `gastown` advertises scaling "comfortably to 20-30 agents". This failure mode is exactly the kind that becomes much more expensive at a 15-agent fan-out:

- Hook queries can fail before work begins, producing false idle or false-empty-hook states.
- Town-vs-rig bead selection errors amplify when multiple agents are reading different prefixes in parallel.
- Dirty or stale installed binaries make branch-level debugging misleading because behavior can diverge from the source tree under inspection.

## Current Conclusion

- The active repair target is the startup/hook control path, not a broad branch merge between the two hook vars.
- `origin/upstream-rebuild-main` already contains substantial routing and Dolt hardening work, but `origin/main` still carries additional fixes that look relevant to the same problem class.
- The highest-confidence explanation for the current blocker is a remaining town-vs-rig routing leak in the hook/control-plane path, combined with a stale installed `gt` binary.
