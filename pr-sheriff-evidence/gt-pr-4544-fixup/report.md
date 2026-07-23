# PR Sheriff Report

Subject: `gastownhall/gastown` PR #4544, `fix: context-check-interval preempts backoff, freezes idle counter`

Original PR: https://github.com/gastownhall/gastown/pull/4544

Final verdict: `merge_replacement`

Merge path allowed: `true`

Replacement branch: `polecat/ghoul/gt-pr-sheriff-4544+371d8a`

Replacement head: `4d651002f278085b77132acc02eb10db78819415`

The original PR identifies a real P1 `kind/bug` affecting await-event backoff/idle progression, but it is conflicting against current main and uses a heuristic context-check threshold. The replacement keeps context-yield semantics, completes already-expired persisted backoff windows immediately through the existing timeout path, and preserves contributor attribution with `Co-authored-by: dog <joshua.guyer@vergesense.com>`.

Evidence gates:

- Research legs: `15/15`
- Pre-implementation reviews: `5/5 approve`
- Post-implementation reviews: `5/5 approve`
- Cleanup-first: `acceptable_minimal`
- Verification: focused await-event tests, compile-only `internal/cmd`, gofmt, and diff check passed
- Checker: `gt-pr-sheriff-check --merge-gate` passed

Required follow-up after replacement lands: close or mark PR #4544 superseded while preserving attribution.
