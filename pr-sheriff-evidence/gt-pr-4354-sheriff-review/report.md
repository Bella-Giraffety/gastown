# PR Sheriff Report

Subject: gastownhall/gastown PR #4354 "fix(daemon): checkpoint_dog fails to unstage nested runtime dirs (ac-d39p9)" https://github.com/gastownhall/gastown/pull/4354

Mode: individual_review / merge_decision

Author: ryanwclark1 (known)

Base/Head: main@5118351294c8e3cad288314b9a9b7d106ebce960 -> e77a26f5aaa651e988b849ff7170004689c56328

Labels: status=accepted priority=p2 kind=bug

Triage category: blocked_or_defer

Action mode: defer_human_review; cleanup-first replacement/fixup recommended

Research legs: 15/15

Pre-implementation reviews: 5/5 approving

Post-implementation reviews: not_applicable; Sheriff did not change PR code

Cleanup-first: acceptable_minimal for the submitted PR, but replacement/fixup should converge root and nested runtime exclusion handling through central runtime policy or Git pathspec/NUL-safe handling rather than expanding bespoke staged-path parsing.

Human approvals: no baseline-red waiver or review approval found.

Verification: focused daemon tests and git repro passed in research; PR CI remains red on Test and Integration Tests.

Baseline-red waiver: not_applicable for non-merge verdict; invalid/missing for any merge path.

Blocker scan: labels, PR body, comments, reviews, CI, and bead scanned. No do-not-merge or changes-requested blocker found. Unresolved merge-path blockers remain status/accepted and red CI.

Gate summary: `gt-pr-sheriff-check --evidence pr-sheriff-evidence/gt-pr-4354-sheriff-review/evidence.json` exited 0 with merge_path_allowed=false, verdict=defer_human_review, gates 6 pass / 6 not_applicable / 0 waived / 2 fail.

Blocking gates: merge_status, blocker_scan.

Final verdict: defer_human_review

Merge path allowed: false

Required next actions:
- Do not merge PR #4354 as-is and do not promote it to review-approved or merge-ready yet.
- Create a maintainer replacement/fixup from clean upstream/main that preserves Ryan Clark attribution and references PR #4354.
- Converge checkpoint runtime exclusion handling by using central runtime artifact policy or Git pathspec/NUL-safe handling for root and nested runtime dirs instead of a bespoke newline-delimited staged-path parser.
- Add an end-to-end checkpointWorktree regression that proves a previously tracked nested `web/.beads/redirect` is excluded while a normal file is checkpointed.
- Rebase/rerun full CI; only consider merge after status becomes review-approved or merge-ready and red checks pass or receive an explicit human baseline-red waiver.

Evidence refs:
- `pr-sheriff-evidence/gt-pr-4354-sheriff-review/evidence.json`
- `pr-sheriff-evidence/gt-pr-4354-sheriff-review/checker.txt`
- `gt-pr-4354-sheriff-review` bead notes/design
