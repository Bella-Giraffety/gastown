# Gastown Startup Evidence - 2026-04-10

This note captures the current startup failures and the supporting evidence gathered from `/tmp` plus live commands run from `gastown/polecats/nitro/gastown`.

## Sources

- `/tmp/coder-startup-script.log`
- `/tmp/gastown-baseline-build.log`
- `/tmp/gastown-baseline-test.log`
- `bd show hq-1jv.3`
- `gt dolt status`
- `gt version --verbose`
- `go version`
- `go env GOTOOLCHAIN GOVERSION`
- `git rev-parse HEAD`
- `ls -ld /home/coder/coder-dotfiles/.beads /home/coder/coder-dotfiles/gastown/mayor/rig/.beads`

## Failure Inventory

1. HQ startup probes were failing because the `hq` database was missing from the startup runtime.

Evidence:

- `/tmp/coder-startup-script.log:216-218` shows an empty non-loadable `hq` bootstrap database being quarantined.
- `/tmp/coder-startup-script.log:223-244` shows repeated startup probe failures for `gt mail inbox mayor/ --all` and `bd show hq-mayor` with `database "hq" not found on Dolt server at 127.0.0.1:3307`.
- `/tmp/coder-startup-script.log:338-345` records the startup runtime using `town_root=/home/coder/gt`, `data_dir=/home/coder/gt/.dolt-data`, and `metadata_db=hq`.

2. Cross-rig `hq-` bead access is still broken from the nitro worktree even though Dolt itself is healthy.

Evidence:

- Live `gt dolt status` reports the server is up on `127.0.0.1:3307` with databases `coder_dotfiles`, `do`, `gastown`, `gs`, and `hq`.
- Live `bd show hq-1jv.3` fails with `PROJECT IDENTITY MISMATCH`.
- `gastown/.beads/metadata.json:4-8` points at Dolt database `gastown` with project id `b0ca637b-e51f-4e5b-b83f-eb4d3b60a38e`.
- `/home/coder/coder-dotfiles/.beads/metadata.json:4-8` points at Dolt database `hq` with project id `5a98cfd9-470c-4b0b-98c0-d7920ab5a539`.
- `/home/coder/coder-dotfiles/.beads/routes.jsonl:1` maps `hq-` to `"path":"."`, so the routing data is sensitive to how the path is resolved.
- `internal/polecat/session_manager_test.go:303-378` documents the intended behavior: `hq-` issues should resolve to the town root, not the rig worktree.

3. Baseline build and test logs are blocked by a Go toolchain mismatch.

Evidence:

- `go.mod:3` requires `go 1.25.8`.
- Live `go version` returns `go1.25.7`.
- Live `go env GOTOOLCHAIN GOVERSION` returns `local` and `go1.25.7`, so the local toolchain cannot auto-upgrade.
- `/tmp/gastown-baseline-build.log:1-3` shows `make build` failing at `Makefile:36` because `go.mod requires go >= 1.25.8`.
- `/tmp/gastown-baseline-test.log:1` shows the same failure for the baseline test run.

4. The installed `gt` binary and the checked-out source are not aligned.

Evidence:

- Live `gt version --verbose` reports `gt version f9bac5a-dirty (dev: polecat/nitro-mntgz40o@f9bac5a)` built with Go `1.25.8`.
- Live `git rev-parse HEAD` in this worktree returns `5c9776364b1d7a87f41ade3ada0d7b326f701999`.
- `git rev-parse` cannot resolve `f9bac5a` in this checkout, so the installed binary was built from a different source state than the current worktree.
- `/tmp/coder-startup-script.log:224` also captured an earlier stale-binary warning: `gt binary is 4 commits behind`.

5. Startup permission warnings are still present.

Evidence:

- Live `ls -ld` shows `/home/coder/coder-dotfiles/.beads` as `drwxrwsr-x` (`0775`).
- Live `ls -ld` shows `/home/coder/coder-dotfiles/gastown/mayor/rig/.beads` as `drwxrws---` (`0770`).
- `gt` and `bd` emit warnings that both should be `0700`.

## Current Assessment

- The original startup failure in `/tmp/coder-startup-script.log` was an `hq` database availability problem under `/home/coder/gt`.
- The current live blocker from this worktree is different: `hq` exists, but `bd` resolves the hooked `hq-` bead through the wrong project context and trips a project identity mismatch.
- Build verification is presently blocked by the host Go toolchain being older than `go.mod`.
- Runtime verification is noisy because the installed `gt` binary does not match the current source checkout.

## Immediate Next Fixes Suggested By The Evidence

1. Fix `hq-` route resolution so town-level beads always resolve against the town root, regardless of worktree cwd.
2. Normalize startup town-root/runtime selection so `/home/coder/gt` and `/home/coder/coder-dotfiles` do not drift.
3. Upgrade or auto-provision Go `1.25.8` for build/test commands when `GOTOOLCHAIN=local` is set.
4. Rebuild and reinstall `gt` from the same checkout being used for verification.
5. Tighten `.beads` permissions to `0700` to remove startup noise.
