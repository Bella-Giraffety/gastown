# Hook And Runtime Cleanup Candidate Inventory

This file is evidence-only. It inventories cleanup candidates in hook installation,
hook sync, runtime integration, and stale compatibility paths without proposing a
combined cleanup branch.

Safety classes used here are provisional:

- `low`: local cleanup with a small semantic-equivalence proof surface
- `medium`: reviewable in one subsystem, but needs call-site inventory or targeted tests
- `high`: touches lifecycle, compatibility, operator-visible behavior, or multi-package fallbacks
- `defer`: not cleanup-safe until a separate migration or approval path exists

Preferred PR classes map to the cleanup contracts in
`cleanup-pr-shape.md`, `cleanup-proof-standards.md`, and
`cleanup-approval-triggers.md`.

## Candidates

| Candidate | Subsystem tag | Cleanup class | Provisional safety class | Preferred PR class | Proof notes needed | Evidence |
| --- | --- | --- | --- | --- | --- | --- |
| Remove the installer-only stale `export PATH=` upgrade branch once no managed hooks need it. | `hooks/install` | stale-compatibility removal | medium | single-subsystem dead-path PR | Prove no shipped template still emits the legacy pattern, inventory whether any still-supported installed hook files rely on installer-time upgrade, and show `gt hooks sync` covers all managed replacements. | `internal/hooks/installer.go:23-30`, `internal/hooks/installer.go:64-72`, `internal/hooks/installer_test.go:96-111` |
| Deduplicate role-aware template lookup names in `roleAwarePatterns()`. The current candidate list can repeat the same filename for `settings.json` and `hooks.json`. | `hooks/install` | duplicate-collapse | low | line-level semantic-equivalence PR | Enumerate all provider/template filename pairs and show that de-duplicating candidate names preserves first-hit behavior and error paths. | `internal/hooks/installer.go:190-233` |
| Collapse the duplicated non-Claude target enumeration shared by `gt hooks sync` and the doctor `hooks-sync` check. | `hooks/sync` | duplicate-collapse | medium | one-subsystem semantic-equivalence PR | Build a complete inventory of role locations, shared-parent vs per-worktree targets, and preserve dry-run/fix output differences while sharing the target discovery logic. | `internal/cmd/hooks_sync.go:117-209`, `internal/doctor/hooks_sync_check.go:92-170` |
| Remove the `CLAUDE_SESSION_ID` fallback from runtime session ID lookup after a full producer/consumer migration. | `runtime/session-id` | stale-compatibility removal | high | dedicated stale-compat PR | Inventory every reader and writer of `CLAUDE_SESSION_ID`, including prime, resume, quota, and preset config; prove non-Claude runtimes still resume correctly when only `GT_SESSION_ID_ENV` is used. | `internal/runtime/runtime.go:64-83`, `internal/cmd/prime.go:80`, `internal/cmd/prime.go:302`, `internal/cmd/prime_session.go:33-55`, `internal/quota/executor.go:174`, `internal/config/agents.go:229`, `internal/config/agents.go:496` |
| Trim or remove the startup fallback path for runtimes without hooks or prompt support only after the runtime capability matrix is re-proven. | `runtime/startup-fallback` | lifecycle cleanup | high | runtime-integration PR with operational evidence | Prove which runtimes still lack hooks or prompt support, preserve startup ordering and nudge timing, and run operational checks through witness/session startup callers before any collapse. | `internal/runtime/runtime.go:85-235`, `internal/witness/manager.go:266-272`, `internal/session/lifecycle.go:162-169` |
| Remove the `commands.IsKnownAgent()` compatibility gate if slash-command provisioning can become unconditional for supported runtimes. | `runtime/settings-provisioning` | invariant enforcement | medium | one-subsystem semantic-equivalence PR | Inventory all supported presets and any intentionally unsupported providers, then prove unconditional provisioning would not write into unknown-agent workdirs or change startup behavior. | `internal/runtime/runtime.go:48-53` |
| Delete the old polecat worktree path fallback after the nested `polecats/<name>/<rig>/` layout is proven universal. | `worktree-layout/compat` | stale-compatibility removal | high | call-site inventory PR | Enumerate every old-layout reader across polecat, witness, deacon, daemon, and doctor code, then prove no supported rig still uses `polecats/<name>/` and that migration coverage exists for any stragglers. | `internal/polecat/session_manager.go:153-175`, `internal/polecat/manager.go:481-503`, `internal/witness/handlers.go:924-931`, `internal/deacon/stale_hooks.go:218-250`, `internal/doctor/worktree_gitdir_check.go:132-143`, `internal/doctor/branch_check.go:457-473` |
| Remove the obsolete pre-checkout compatibility shim only after the branch-protection rename is fully retired. | `doctor/branch-protection` | stale-compatibility removal | medium | small stale-compat PR | Prove no internal registration still needs `NewPreCheckoutHookCheck` or the alias type, and keep the on-disk hook migration logic until old installs are no longer expected. | `internal/doctor/precheckout_hook_check.go:33-36`, `internal/doctor/precheckout_hook_check.go:93-109`, `internal/doctor/precheckout_hook_check.go:174-179`, `internal/doctor/precheckout_hook_check.go:221-222`, `internal/cmd/doctor.go:180`, `internal/cmd/upgrade.go:130` |
| Remove the `GT_HOME` read fallback to `~/.gt` only with an explicit config-home migration. | `hooks/config-home` | stale-compatibility removal | defer | preserve-or-migrate first, cleanup second | Prove the operator contract for `GT_HOME`, inventory other path helpers and telemetry/costs writers, and add a migration story before changing read precedence. | `internal/hooks/config.go:656-746`, `internal/cmd/paths.go:12-16`, `internal/cmd/telemetry.go:13-24`, `internal/cmd/costs.go:932` |

## Slicing Notes

- The lowest-risk candidate is `roleAwarePatterns()` deduplication because the
  proof surface stays inside template name resolution.
- The highest-risk group is the runtime and layout compatibility set:
  `CLAUDE_SESSION_ID` fallback removal, startup fallback removal, old polecat
  path removal, and `GT_HOME` precedence cleanup all cross lifecycle or operator
  contracts called out in `cleanup-approval-triggers.md`.
- The hook sync duplication candidate is promising, but it should stay within the
  hook-sync subsystem and avoid mixing target discovery cleanup with any behavior
  changes to sync output, target counting, or integrity handling.

## Explicit Follow-up Records

- `runtime/session-id` -> `gt-djp.7.3.1`
- `runtime/startup-fallback` -> `gt-djp.7.3.2`
- `worktree-layout/compat` -> `gt-djp.7.3.3`
- `hooks/config-home` -> `gt-djp.7.3.4`
