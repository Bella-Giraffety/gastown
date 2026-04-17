# Startup And Bootstrap Cleanup Candidate Inventory

This file is evidence-only. It inventories startup and bootstrap cleanup
candidates by comparing release-era survivorship with the current authoritative
startup path. It does not propose a combined cleanup branch.

Safety classes used here are provisional:

- `low`: local cleanup with a small semantic-equivalence proof surface
- `medium`: reviewable in one subsystem, but needs call-site inventory or
  existing-test coverage rationale
- `high`: touches startup ordering, hook-failure fallback, operator-visible
  bootstrap files, or multi-role lifecycle behavior
- `defer`: not cleanup-safe until a separate migration or approval path exists

Preferred PR classes map to the cleanup contracts in
`cleanup-pr-shape.md`, `cleanup-proof-standards.md`, and
`cleanup-approval-triggers.md`.

## Candidates

| Candidate | Subsystem tag | Cleanup class | Provisional safety class | Preferred PR class | Disposition | Proof notes needed | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Consolidate town-root identity-anchor writers so install, upgrade, and doctor stop carrying separate `CLAUDE.md` creation logic. | `bootstrap/town-root-anchor` | duplicate-collapse | medium | one-subsystem semantic-equivalence PR | independent | Inventory every writer of town-root `CLAUDE.md` and `AGENTS.md`, then collapse onto the embedded template path while preserving the current AGENTS symlink contract and the doctor behavior for missing-vs-incomplete files. | `internal/cmd/install.go:503-549`, `internal/cmd/upgrade.go:225-252`, `internal/doctor/priming_check.go:425-431`, `internal/doctor/town_claude_md_check.go:98-147`, `internal/templates/townroot.go:13-20` |
| Remove the legacy sparse-checkout remediation path once no supported rigs still depend on the old `.claude/`/`CLAUDE.md` exclusion pattern. | `doctor/legacy-sparse-bootstrap` | stale-compatibility removal | medium | small stale-compat PR | independent | Prove current `gt rig add` installs and current docs no longer rely on the legacy exclusion pattern, and distinguish old auto-generated sparse checkouts from intentionally user-requested sparse checkouts so the cleanup does not remove a supported feature. | `internal/doctor/sparse_checkout_check.go:11-179`, `internal/rig/manager.go:519-525`, `docs/reference.md:381-399`, `docs/design/architecture.md:131-133` |
| Delete the stale intermediate-directory `CLAUDE.md`/`AGENTS.md` cleanup path after the no-per-directory bootstrap model is proven universal. | `priming/intermediate-files` | stale-compatibility removal | medium | small stale-compat PR | independent | Inventory every code path that still creates or repairs intermediate instruction files, show new installs and upgrades only rely on the town-root anchor plus hooks, and preserve customer repo instruction files inside real worktrees. | `internal/doctor/priming_check.go:85-99`, `internal/doctor/priming_check.go:238-255`, `internal/doctor/priming_check.go:448-459`, `internal/rig/manager.go:618-620`, `docs/reference.md:368-377` |
| Remove per-worktree `PRIME.md` provisioning only after SessionStart-hook failure recovery is re-proven without it. | `priming/worktree-prime-fallback` | stale-bootstrap removal | high | priming fallback PR with failure-path evidence | deferred | Prove which roles still need `bd prime` as a hook-failure fallback, characterize startup behavior when hooks are missing or broken, and preserve redirect-aware provisioning semantics so worktrees do not regress into local orphan `.beads/PRIME.md` files. | `internal/beads/beads.go:1658-1692`, `internal/polecat/manager.go:975-980`, `internal/doctor/priming_check.go:221-333`, `internal/doctor/priming_check.go:441-445`, `CHANGELOG.md:1215-1217` |
| Remove the polecat `CLAUDE.md`/`CLAUDE.local.md` lifecycle overlay only if `gt prime` plus the town-root/customer instruction chain is proven sufficient for completion behavior across spawn, reuse, and compaction. | `polecat/bootstrap-overlay` | lifecycle cleanup | high | approval-gated lifecycle PR | approval-gated | Reconcile the current authority model first: docs say only the town-root anchor exists, but polecat spawn and reuse still provision a local lifecycle overlay and git-ignore it. Any cleanup must prove `gt done` and completion protocol reminders still survive compaction, reuse, and non-Claude runtimes without relying on the local overlay. | `internal/templates/templates.go:215-268`, `internal/polecat/manager.go:956-964`, `internal/polecat/manager.go:1639-1645`, `internal/rig/overlay.go:14-23`, `docs/design/architecture.md:131-133`, `CHANGELOG.md:177-179` |
| Collapse witness/refinery custom startup orchestration into the shared session lifecycle only after extracting the genuinely role-specific post-start steps. | `session/startup-managers` | duplicate-collapse | high | sequenced startup-lifecycle PRs | sequenced | First inventory the unique steps that still justify custom managers: role-config env injection, `GT_REFINERY`, nudge poller startup, and prompt-fallback ordering. Then migrate one role at a time onto `session.StartSession` while proving startup dialogs, readiness waits, fallback nudges, and telemetry ordering remain equivalent. | `internal/session/lifecycle.go:21-143`, `internal/session/lifecycle.go:161-305`, `internal/witness/manager.go:155-287`, `internal/refinery/manager.go:167-281` |

## Slicing Notes

- The cleanest independent slices are the town-root identity-anchor writer
  collapse and the two doctor-only stale-compatibility paths.
- `PRIME.md` fallback removal is not routine cleanup today; it is still part of
  the documented hook-failure recovery story, so it should stay deferred until
  that fallback contract is replaced or re-proven.
- The polecat local `CLAUDE` overlay is the main approval-gated candidate
  because current docs and current code disagree about whether local bootstrap
  files still exist, which means cleanup cannot rely on doc intent alone.
- Witness/refinery startup deduplication looks promising, but it should be
  sequenced by role and must avoid pulling the much more specialized polecat
  startup path into the same slice.
