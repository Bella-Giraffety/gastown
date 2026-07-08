# PR Sheriff Report

Subject: `gastownhall/gastown` PR #4426 `fix(formula): converge formula root resolution`
URL: https://github.com/gastownhall/gastown/pull/4426
Mode: merge decision refresh / non-merge gate
Author: `Bella-Giraffety`
Base/Head: `main@0220c910de595ac617dff7d601a9d263fc87fb7b` -> `b0467e95fa21b020bf6dfcde2ae2eb9b72905944`
Current main: `3b9c2bf73939bad7c6744eddf12d0c5882923cc7`
Labels: status=`reviewing` priority=`p2` kind=`bug`

Final verdict: `defer_human_review`
Merge path allowed: `false`

Blocking gates:
- `merge_status`: PR #4426 is still `status/reviewing`, not `status/review-approved` or `status/merge-ready`.
- `blocker_scan`: no GitHub review approval exists; PR checks are stale against current `main`; prior gt-6uao evidence intentionally deferred merge; route-resolved formula lookup needs containment review/fix before promotion.

Evidence summary:
- Research legs: 15/15 complete.
- Pre-decision reviews: 5/5 approving after adding non-merge checker validation.
- Post-implementation reviews: not applicable for this refresh; this Sheriff pass did not change code or mutate the PR.
- Cleanup-first: convergent. PR #4426 removes duplicate formula lookup paths and routes named formula loading through the shared resolver.
- Verification: PR head checks pass, but `main` advanced five commits after the checked merge base, so final merge readiness requires refresh/rebase and rerun.
- Replacement flow: #4426 preserves attribution to original #4334 author Anthony Wong / `antiwong`; #4334 should remain open until #4426 lands.

Checker commands:
- `gt-pr-sheriff-check --evidence pr-sheriff-evidence/gt-2ota-pr4426-merge-gate/evidence.json` -> PASS, `merge_path_allowed=false`, `verdict=defer_human_review`.
- `gt-pr-sheriff-check --evidence pr-sheriff-evidence/gt-2ota-pr4426-merge-gate/evidence.json --merge-gate` -> BLOCK, `merge_path_allowed=false`.

Required next actions:
- Keep PR #4426 open and do not merge yet.
- Resolve or explicitly accept the route path containment concern in formula resolution.
- Refresh/rebase #4426 onto current `main` and rerun CI before any merge-ready claim.
- Obtain maintainer review and promote status only after blockers are resolved.
- Keep #4334 open until #4426 lands, then close/supersede it with attribution.
