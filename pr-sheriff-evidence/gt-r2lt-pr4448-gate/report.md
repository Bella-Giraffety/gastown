# PR Sheriff Report

Subject: `gastownhall/gastown` PR #4448 `fix(formula): converge formula root resolution`
URL: https://github.com/gastownhall/gastown/pull/4448

Mode: merge decision gate, non-merge defer
Author: `Bella-Giraffety`
Base/Head: `main@68320ca2298faafe25f27cfcb8f31ecd6eb1873a` -> `2f8634ec64a40805e69d7f664864baa1c1ac8cd7`
Labels: `status=reviewing`, `priority=p2`, `kind=bug`

Final verdict: `defer_human_review`
Merge path allowed: `false`

## Blocking Gates

- `merge_status`: PR is still `status/reviewing`, with no visible maintainer approval or review decision.
- `blocker_scan`: current blockers remain unresolved: existing PR-committed Sheriff evidence/checker is non-merge and stale versus current head; branch lacks current-main merge-result verification; review found a likely `gt formula create` versus named `gt formula run` root mismatch; Codecov reports low patch coverage.

## Evidence Summary

- 15 independent research legs completed.
- 5 approving pre-decision reviews completed for the updated current-head defer action.
- No Sheriff code/fixup/replacement/cherry-pick was performed.
- Cleanup-first assessment: `convergent` in direction, but not ready for merge.
- Observed current PR-head CI checks passed: Test, Lint, Integration Tests, Windows Smoke Test, reject guards, and remove-label.
- Checker command passed without `--merge-gate`: `gt-pr-sheriff-check --evidence pr-sheriff-evidence/gt-r2lt-pr4448-gate/evidence.json`.

## Required Next Actions

- Obtain maintainer review and promote status to `status/review-approved` or `status/merge-ready` before any merge path.
- Refresh PR Sheriff evidence/checker to the current target head before any future merge-gate run.
- Rebase or refresh against current `upstream/main` and rerun CI, or provide valid baseline-red waiver evidence for unrelated failures.
- Resolve or explicitly accept the `gt formula create` versus named `gt formula run` root mismatch before merge readiness.
- Review the Codecov low patch coverage warning and add focused coverage or record maintainer acceptance.

Evidence refs:

- `pr-sheriff-evidence/gt-r2lt-pr4448-gate/evidence.json`
- `pr-sheriff-evidence/gt-r2lt-pr4448-gate/checker.txt`
