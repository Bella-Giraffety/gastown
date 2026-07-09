# PR Sheriff Report

Subject: `gastownhall/gastown` PR #4346 `fix: guard stuck-agent mass-death escalation`  
URL: https://github.com/gastownhall/gastown/pull/4346  
Mode: merge decision, review-only/no code change  
Author: `Bella-Giraffety` (trusted/maintainer-adjacent)  
Base/Head Checked: `upstream/main@81233d36f27465fcae83944f78bdea1674d7143e` -> PR head `ff2f53f38bb93183abac829e4fc127670acb6b0e`  
Evidence: `pr-sheriff-evidence/gt-pr-4346-stuck-agent-mass-death/evidence.json`

## Final Verdict

`defer_human_review`

`merge_path_allowed: false`

Do not merge, promote status, or route PR #4346 through a merge path from this run.

## Blocking Gates

- `merge_status`: PR #4346 is still labeled `status/reviewing`, not `status/review-approved` or `status/merge-ready`; no visible GitHub review, approval, or status-promotion artifact exists.
- `blocker_scan`: unresolved blocker evidence remains: stale PR body evidence, stale docs-contract concern, and current-main verification freshness risk.
- `final_verdict` under `--merge-gate`: the valid current verdict is `defer_human_review`, not `merge_as_is` or `merge_replacement`.

## Checker Results

Non-merge validation passed:

```text
Command: gt-pr-sheriff-check --evidence pr-sheriff-evidence/gt-pr-4346-stuck-agent-mass-death/evidence.json
Exit code: 0

PR Sheriff: PASS
merge_path_allowed: false
verdict: defer_human_review
gates: 5 pass, 7 not_applicable, 0 waived, 2 fail
```

Merge gate blocked:

```text
Command: gt-pr-sheriff-check --evidence pr-sheriff-evidence/gt-pr-4346-stuck-agent-mass-death/evidence.json --merge-gate
Exit code: 1

PR Sheriff: BLOCK
merge_path_allowed: false
verdict: defer_human_review
gates: 5 pass, 6 not_applicable, 0 waived, 3 fail
blocking_gates:
- merge_status: status reviewing is not merge-ready; merge-ready status is contradicted by unresolved blocker evidence
- blocker_scan: unresolved blockers found
- final_verdict: --merge-gate requires final.verdict merge_as_is or merge_replacement
```

## Evidence Snapshot

- PR metadata: open, non-draft, head `ff2f53f38bb93183abac829e4fc127670acb6b0e`, labels exactly `kind/bug`, `priority/p0`, `status/reviewing`, empty `reviewDecision`, and no visible reviews.
- Diff scope: 3 files under `plugins/stuck-agent-dog`: `plugin.md`, `run.sh`, `run_test.sh`; 425 additions and 113 deletions.
- Current main: `81233d36f27465fcae83944f78bdea1674d7143e`; PR merge base remains `1bae8a3ca9f8693ea70bb2fbd101f87a8882d0eb`.
- Clean apply evidence: `git merge-tree --write-tree upstream/main refs/remotes/upstream/pr/4346` exited 0 and produced tree `596b85446300ff1b721123b74f8548fe93a3f84e`.
- Focused verification: detached PR-head worktree passed `git diff --check upstream/main...HEAD`, `bash -n plugins/stuck-agent-dog/run.sh plugins/stuck-agent-dog/run_test.sh`, and `bash plugins/stuck-agent-dog/run_test.sh` with `83 passed, 0 failed`.
- GitHub PR checks: visible PR head checks pass for `ff2f53f38bb93183abac829e4fc127670acb6b0e`, but they are from 2026-06-29.
- Current-main freshness risk: latest current-main CI run `29033718103` for `81233d36f27465fcae83944f78bdea1674d7143e` is red, so merge-result freshness is not established.
- Cleanup-first assessment: executable behavior is convergent; it replaces stale registry/bead-status decision paths with live rig registry, live hook state, and central health recheck.

## Research Legs

- `research-pr4346-metadata`: merge path blocked by `status/reviewing`, no reviews/approval, and stale base/check evidence.
- `research-pr4346-problem-fit`: PR addresses the false mass-death escalation by revalidating live central health and hook state before CRITICAL escalation.
- `research-pr4346-scope`: code scope is narrow, but PR body remains stale because it claims two commits while the PR has three and is behind current main.
- `research-pr4346-cleanup`: runtime change is convergent and removes stale authority paths; no runtime bandaid found.
- `research-pr4346-correctness`: live rig filtering, hook status authority, recheck behavior, threshold defaulting, and fail-closed paths are acceptable.
- `research-pr4346-tests`: focused tests cover active hooks, idle/no-hook agents, non-actionable hook states, docked rigs, fail-closed rig list, recovered candidates, cleared hooks, downgrade behavior, and mass-death metadata.
- `research-pr4346-security`: no new secrets, auth, dependency, CI, logging, or command-injection risk found.
- `research-pr4346-docs`: blocker; `plugin.md` still documents legacy `rigs.json`/`RIGS_JSON_PATH` copied-script fallback behavior that `run.sh` no longer implements.
- `research-pr4346-ci`: PR head checks are green, but stale relative to current main and current-main CI is red.
- `research-pr4346-blockers`: no new do-not-merge/security blocker found, but prior Sheriff defer blockers remain unresolved.
- `research-pr4346-attribution`: current PR preserves commit authorship; future replacement/fixup would need public attribution preservation.
- `research-pr4346-current-main`: PR applies cleanly to current upstream/main and is not superseded.
- `research-pr4346-shell`: bash reliability is acceptable; executable lack of `rigs.json` fallback is a docs/product-contract issue, not a shell blocker.
- `research-pr4346-operational`: operational impact is positive but fail-closed live-state unavailability can delay detection for a cycle.
- `research-pr4346-decision`: current evidence supports only `defer_human_review`, not merge.

## Pre-Decision Reviews

- `pre-pr4346-policy`: approved defer because status/approval/current-main/stale-evidence blockers remain.
- `pre-pr4346-cleanup`: approved defer as the cleanup-first minimal action; replacement/fixup now would add churn without clearing policy blockers.
- `pre-pr4346-verification`: approved defer because green PR-head checks do not substitute for status promotion or fresh current-main merge evidence.
- `pre-pr4346-risk`: approved defer because watchdog restart/escalation behavior has availability blast radius and stale blockers should not be bypassed.
- `pre-pr4346-evidence`: approved evidence shape for non-merge checker validation with `merge_status` and `blocker_scan` failures.

## Required Next Actions

1. A configured maintainer/reviewer must resolve or explicitly accept the stale PR body commit-count/current-main evidence and the `plugin.md` fallback-docs contract concern.
2. If approved, promote PR #4346 to `status/review-approved` or `status/merge-ready`.
3. Refresh verification against current main, or provide a valid explicit baseline-red waiver if current main remains red.
4. Rerun `gt-pr-sheriff-check --evidence <file> --merge-gate` against unchanged head `ff2f53f38bb93183abac829e4fc127670acb6b0e`, or recollect full evidence if the head changes.
