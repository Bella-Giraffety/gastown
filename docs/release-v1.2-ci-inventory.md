# gt v1.2 CI Inventory

Snapshot time: 2026-05-23 14:45 UTC
Repository: `gastownhall/gastown`
Release coordination bead: `gt-12-baseline-ci-inventory`
Release staging branch: `integration/test-beaddolt-hardenning`

## Review Protocol

Pre-implementation review passes were recorded before this file was added:

1. Actions baseline pass: inspected latest `main` CI, Windows, E2E, Nightly, and Release workflow runs.
2. Workflow trigger pass: inspected `.github/workflows/ci.yml`, `.github/workflows/windows-ci.yml`, `.github/workflows/e2e.yml`, `.github/workflows/nightly-integration.yml`, and `.github/workflows/release.yml`.
3. PR rollup pass: inspected open PR status rollups for `main` and `integration/test-beaddolt-hardenning` targets.
4. Baseline-vs-branch pass: separated failures already present on `main` from PR-only failures.
5. Release policy pass: classified which failures can be waived for PR merge and which block the final tag.

Post-implementation review passes were recorded after this file was drafted:

1. Acceptance-criteria pass: verified branch-owned failures, baseline failures, and final tag gates are all explicitly represented.
2. Link pass: verified each baseline failure row has an Actions run link and a job link.
3. Workflow-scope pass: verified the release-staging CI coverage gap is called out separately from test failures.
4. Waiver-policy pass: verified PR-merge waivers and final-tag blockers are distinct and actionable.
5. Scope pass: verified this change is documentation-only and does not alter CI behavior or release mechanics.

## Workflow Coverage

| Workflow | Trigger | Required signal for v1.2 | Current status |
| --- | --- | --- | --- |
| `CI` | `push` to `main`, `pull_request` to `main` | `Lint`, `Test`, `Integration Tests`, plus PR-only `Reject go.mod replace directives` and `Reject issues.jsonl` | Red on latest `main` push |
| `Windows CI` | `push` to `main`, `pull_request` to `main` | `Windows Smoke Test` | Green on latest `main` push |
| `E2E Tests` | daily schedule, manual dispatch | `E2E Tests (Container)` | Red on latest scheduled `main` run |
| `Nightly Integration Tests` | daily schedule, manual dispatch | `Full Integration Tests` | Red on latest scheduled `main` run |
| `Release` | `v*` tag push, manual dispatch | `goreleaser`, `attest-release`, `update-homebrew-formula`; `publish-npm` is best-effort by workflow design | Last release run was green for `v1.1.0` |

Important gap: PR CI for `CI` and `Windows CI` currently targets `main` only. PRs targeting `integration/test-beaddolt-hardenning` do not get the normal `Lint`, `Test`, `Integration Tests`, or `Windows Smoke Test` rollup unless manually retargeted or dispatched through another gate. That means release-staging PRs can look clean while missing release-relevant checks.

## Current Baseline Failures

Baseline SHA: `625bcf8a92f9faef9804f73624a8bf770085ebd2` on `main`.

| Area | Run | Job link | Exact failures | Classification |
| --- | --- | --- | --- | --- |
| Lint | [CI run 26141061089](https://github.com/gastownhall/gastown/actions/runs/26141061089) | [Lint job 76886495536](https://github.com/gastownhall/gastown/actions/runs/26141061089/job/76886495536) | `internal/cmd/statusline.go:114:26: runWorkerStatusLine - t is unused (unparam)`; `internal/cmd/statusline.go:441:28: runRefineryStatusLine - t is unused (unparam)` | Baseline-red |
| Test | [CI run 26141061089](https://github.com/gastownhall/gastown/actions/runs/26141061089) | [Test job 76886495557](https://github.com/gastownhall/gastown/actions/runs/26141061089/job/76886495557) | `internal/cmd.TestFilterAndSortSessions_SortOrder`; `internal/cmd.TestGuessSessionFromWorkerDir/different_rig`; `internal/cmd.TestGuessSessionFromWorkerDir`; `internal/cmd.TestJSONOutput_NoHumanReadableText`; `internal/cmd.TestJSONOutput_ErrorsReturnNonZeroExit`; `internal/polecat.TestReuseIdlePolecat_KillsLiveSession`; `internal/polecat.TestReuseIdlePolecat_KillsStaleSession` | Baseline-red |
| Integration | [CI run 26141061089](https://github.com/gastownhall/gastown/actions/runs/26141061089) | [Integration job 76886495549](https://github.com/gastownhall/gastown/actions/runs/26141061089/job/76886495549) | `internal/cmd.TestFilterAndSortSessions_SortOrder`; `internal/cmd.TestGuessSessionFromWorkerDir/different_rig`; `internal/cmd.TestGuessSessionFromWorkerDir`; `internal/cmd.TestJSONOutput_NoHumanReadableText`; `internal/cmd.TestJSONOutput_ErrorsReturnNonZeroExit` | Baseline-red |
| Windows | [Windows run 26141061077](https://github.com/gastownhall/gastown/actions/runs/26141061077) | [Windows Smoke Test job](https://github.com/gastownhall/gastown/actions/runs/26141061077) | None | Green baseline |
| E2E | [E2E run 26326172833](https://github.com/gastownhall/gastown/actions/runs/26326172833) | [E2E job 77504209905](https://github.com/gastownhall/gastown/actions/runs/26326172833/job/77504209905) | `TestInstallCreatesCorrectStructure`; `TestInstallBeadsHasCorrectPrefix`; `TestInstallFormulasProvisioned` was reached and failed with the same cause before the captured log truncated. Cause: E2E image installs Dolt `1.82.4`, but `gt install` now requires minimum Dolt `1.84.0`. | Baseline-red |
| Nightly | [Nightly run 26326340588](https://github.com/gastownhall/gastown/actions/runs/26326340588) | [Full Integration Tests job 77504669304](https://github.com/gastownhall/gastown/actions/runs/26326340588/job/77504669304) | 20 failures, including `internal/beads.TestInitPassesServerFlag`, `internal/cmd.TestFilterAndSortSessions_SortOrder`, `internal/cmd.TestGuessSessionFromWorkerDir/different_rig`, `internal/cmd.TestGuessSessionFromWorkerDir`, `internal/cmd.TestJSONOutput_NoHumanReadableText`, `internal/cmd.TestJSONOutput_ErrorsReturnNonZeroExit`, `internal/cmd.TestFreshInstallRigPolecatHookIntegration`, multiple `TestScheduler*` failures caused by `bead ... is not present in target rig`, `internal/polecat.TestReuseIdlePolecat_KillsLiveSession`, and `internal/polecat.TestReuseIdlePolecat_KillsStaleSession` | Baseline-red |

## Branch-Owned PR Failures

Release-staging PRs targeting `integration/test-beaddolt-hardenning` currently show no branch-owned `Lint`, `Test`, `Integration Tests`, or `Windows Smoke Test` failures because those workflows do not run against that base branch. The visible checks on PRs `#4084` through `#4092` are label/internal-policy checks only, so they are insufficient as release gates.

Open `main` PRs with failures that exceed, or may exceed, the current baseline:

| PR | Head branch | Failed checks | Branch-owned assessment |
| --- | --- | --- | --- |
| [#4081](https://github.com/gastownhall/gastown/pull/4081) | `fix/polecat-cap-admission-4075` | `Windows Smoke Test`, `Test`, `Lint`, `Integration Tests` | Windows is branch-owned until proven otherwise because latest `main` Windows baseline is green. Linux failures overlap the red baseline but still require diff-aware comparison before merge. |
| [#4066](https://github.com/gastownhall/gastown/pull/4066) | `claude/fix-formula-prime-overlay-and-issue-var` | `Test` | Branch-owned until rerun against current `main`; the failed run predates the latest baseline snapshot and only failed one job. |
| [#4105](https://github.com/gastownhall/gastown/pull/4105) | `fix/autonomous-suppress-survey-recap` | `Test`, `Lint`, `Integration Tests` | Looks baseline-like because failed job set matches latest `main` CI failure set. Waivable for PR merge only if test names match the baseline list above and diff does not touch owning areas. |
| [#4097](https://github.com/gastownhall/gastown/pull/4097) | `polecat/shiny/main-status-mail-collapse` | `Test`, `Lint`, `Integration Tests` | Looks baseline-like by job set. Waivable only with exact failure-name match and diff check. |

## Merge And Release Policy

PR merge may waive a check only when all of the following are true:

1. The failed job and exact failing test names match this inventory's baseline-red list.
2. The branch does not modify files that own the failing behavior.
3. The PR has no new green-to-red signal relative to `main`; Windows failures are not waivable while the Windows baseline is green.
4. The waiver is recorded in the merge request or release coordination bead with run links and rationale.

PR merge must not waive:

1. `Reject go.mod replace directives`.
2. `Reject issues.jsonl`.
3. Any new `Lint` failure in files touched by the PR.
4. Any `Windows Smoke Test` failure while `main` Windows is green.
5. Any release workflow failure on a `v*` tag except the explicitly best-effort `publish-npm` job.

Final `v1.2.0` tag gate:

1. `CI` on `main` must be green for `Lint`, `Test`, and `Integration Tests`, or each remaining baseline-red failure must have an explicit release-owner waiver with links and a rationale.
2. `Windows CI` on `main` must be green. No current waiver is justified because the latest `main` Windows run is green.
3. Scheduled or manually-dispatched `E2E Tests` on the release SHA must be green, or the Dolt version mismatch in the E2E image must be formally waived as infrastructure-only. Prefer fixing the image to install Dolt `>=1.84.0`.
4. Scheduled or manually-dispatched `Nightly Integration Tests` on the release SHA must be green, or all 20 failures must have owner-approved waivers. Current failures are too broad to silently waive.
5. `make check-version-tag` must pass on the `v1.2.0` tag.
6. The `Release` workflow must pass `goreleaser` and `attest-release`; `publish-npm` may fail without blocking because `.github/workflows/release.yml` sets `continue-on-error: true` for that job.

## Immediate Blockers

1. Fix CI workflow coverage for `integration/test-beaddolt-hardenning` PRs, or require manual dispatch/equivalent gate runs before treating release-staging PRs as verified.
2. Fix latest `main` lint failures in `internal/cmd/statusline.go`.
3. Fix or explicitly waive the `Test`/`Integration` baseline failures listed above.
4. Update E2E Dolt installation from `1.82.4` to `>=1.84.0` or formally waive the E2E image mismatch before final tag.
5. Triage Nightly's 20 failures before final tag; the scheduler and polecat reuse failures overlap release-critical agent dispatch behavior and should not be silently waived.
