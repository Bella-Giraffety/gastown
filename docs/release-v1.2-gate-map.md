# gt 1.2 Release Gate Map

Last reviewed: 2026-05-23

This map keeps the GitHub milestone `gt 1.2 release gate` aligned with the
local release epic `gt-1-2-release-gate`. The milestone must contain the
release-blocking GitHub issues/PRs and omit PRs that are only historical or
superseded by a canonical replacement.

## Pre-Implementation Review Passes

1. Milestone membership pass: the milestone already contained #4073-#4079 and
   #4080/#4081/#4096; active successor PRs #4110-#4114 were missing and were
   added during this review.
2. Superseded-only pass: #4085/#4086/#4087/#4088/#4089/#4092 are excluded from
   the milestone because they are diagnostic, superseded, or partially ported
   integration/test PRs with explicit replacement paths.
3. Local epic pass: each release track has a canonical local owner epic so
   release decisions can be tied back to beads, not just GitHub PR state.
4. Blocker pass: unresolved source issues, red CI on upstream-main PRs, and
   unlanded successor PRs remain release blockers until landed or explicitly
   deferred.
5. Evidence pass: every release decision below cites the GitHub item, canonical
   local owner, replacement/successor path, and current gating evidence.

## Canonical Track Map

| Track | Source issue | Canonical local owner | Replacement or successor PR | Release classification | Decision evidence |
| --- | --- | --- | --- | --- | --- |
| Overall polecat re-enable convergence | #4073 | `gt-1-2-release-gate` | Subepic integration branches; #4080/#4081/#4096 and #4110-#4114 are active GitHub evidence | Blocker | Parent issue remains open; release cannot cut until all subepic blockers land or are formally deferred. |
| Polecat lifecycle/workstate | #4074 | `gt-12-polecat-workstate` | #4080 remains the main-target PR evidence; final canonical path is `integration/gt-1-2-canonical-polecat-workstate-workstate` | Blocker until landed or superseded | Local epic children are complete/eligible for close, but #4080 is still open with red Test/Lint/Integration checks. |
| Capacity and admission | #4075 | `gt-12-capacity-admission` | #4081 plus successor #4111 for the #4087 recovery-slot fold | Blocker | `gt-12-admission-path-tests` remains open; #4081 is open with red Windows/Test/Lint/Integration checks; #4111 is clean but unmerged. |
| MR target and source transitions | #4076/#4077 | `gt-12-mr-target-source` | Final upstream PR pending from `integration/gt-1-2-mr-target-and-source-transition-gate-source` | Blocker until final PR or explicit deferral | Local epic children are complete/eligible for close, but no milestone GitHub PR currently represents the final subepic branch. |
| Routing identity | #4096 | `gt-12-routing-identity` | #4096 replaces #4086/#4088/#4092; successor #4110 covers formula identity tests | Blocker | Local epic children are complete/eligible for close; #4096 is open with red Test/Integration checks; #4110 is clean but unmerged. |
| Notification actionability | #4078 | `gt-12-notification-actionability` | Successor #4112 | Blocker until landed | Local epic children are complete/eligible for close; #4112 is clean but unmerged. |
| Recovery false positives | #4073/#4079 support track | `gt-12-recovery-false-positives` | Successor #4113 covers live polecat fixture evidence | Blocker until landed | Local epic children are complete/eligible for close; #4113 is clean but unmerged. |
| Release candidate and canary | #4079 | `gt-12-release-candidate-canary` | No final PR yet; canary/build/quality/cut beads remain open | Blocker | Only current-polecat classification is complete; build/install RC, canary re-enable, release quality gates, and cut release remain open. |
| Reuse/session-startup remainder | #4073/#4074 prior-art slice | `gt-12-polecat-workstate` plus superseded PR map tracker | Successor #4114 for `gt-pr-main-4089-reuse-startup-fold` | Blocker until landed or explicitly deferred | #4114 is clean but targets `integration/test-beaddolt-hardenning`; release owner must decide whether it lands, is folded into a subepic PR, or is deferred. |
| Coordination inventory | #4073 support track | `gt-12-coordination-inventory` | This document; superseded map and CI inventory beads | Non-code blocker evidence | Coordination remains open until this map, CI inventory, and superseded PR dispositions are current. |

## Milestone Contents

Required GitHub issues in the milestone:

| Issue | Title | Classification | Canonical owner | Release decision |
| --- | --- | --- | --- | --- |
| #4073 | Maintenance: converge polecat lifecycle and re-enable safe dispatch | Blocker | `gt-1-2-release-gate` | Keep open until all source-fix and canary tracks land or are explicitly deferred. |
| #4074 | Centralize polecat lifecycle verdict and reuse eligibility | Blocker | `gt-12-polecat-workstate` | Keep open while #4080/final workstate branch is unresolved. |
| #4075 | Enforce configured polecat cap at central spawn and reuse admission | Blocker | `gt-12-capacity-admission` | Keep open while #4081/#4111 and remaining admission tests are unresolved. |
| #4076 | Centralize MR target resolution and MR creation safety | Blocker | `gt-12-mr-target-source` | Keep open until the final target/source PR lands or is explicitly deferred. |
| #4077 | Centralize source issue transitions and completion recovery | Blocker | `gt-12-mr-target-source` | Keep open until the final target/source PR lands or is explicitly deferred. |
| #4078 | Make notifications actionability-based without suppressing real alerts | Blocker | `gt-12-notification-actionability` | Keep open until #4112/final notification branch lands. |
| #4079 | Classify existing polecats and canary re-enable configured capacity | Blocker | `gt-12-release-candidate-canary` | Keep open until classification, RC install, quality gates, canary, and release cut gates are complete. |

Required GitHub PRs in the milestone:

| PR | Title | Source issue/track | Owner | Classification | Release decision evidence |
| --- | --- | --- | --- | --- | --- |
| #4080 | fix: centralize polecat workstate reuse verdict | #4074 | `gt-12-polecat-workstate` | Blocker | Open, `UNSTABLE`, red Test/Lint/Integration checks. Keep until landed or replaced by final subepic PR. |
| #4081 | fix: enforce polecat cap admission | #4075 | `gt-12-capacity-admission` | Blocker | Open, `UNSTABLE`, red Windows/Test/Lint/Integration checks. Keep until landed or replaced by final subepic PR. |
| #4096 | fix: rebuild routing convergence for main | Routing identity | `gt-12-routing-identity` | Blocker | Open, replaces #4086/#4088/#4092, red Test/Integration checks. |
| #4110 | Merge: gt-12-formula-identity-tests | Routing identity | `gt-12-routing-identity` | Blocker until merged | Clean successor PR for formula identity coverage. |
| #4111 | Merge: gt-12-fold-4087-capacity | Capacity/admission | `gt-12-capacity-admission` | Blocker until merged | Clean successor PR folding #4087 recovery-slot capacity accounting. |
| #4112 | Merge: gt-12-notification-regression-tests | Notification actionability | `gt-12-notification-actionability` | Blocker until merged | Clean successor PR for actionability regression coverage. |
| #4113 | Merge: gt-12-live-polecat-fixtures | Recovery false positives | `gt-12-recovery-false-positives` | Blocker until merged | Clean successor PR for live fixture evidence. |
| #4114 | Merge: gt-pr-main-4089-reuse-startup-fold | Reuse/startup remainder | `gt-12-polecat-workstate` / superseded PR tracker | Blocker until merged or deferred | Clean successor PR for the remaining #4089 startup/reuse slice. |

## Excluded Superseded or Diagnostic PRs

These PRs should stay out of the milestone unless a maintainer reclassifies one
as an active release blocker:

| PR | Disposition | Replacement or rationale |
| --- | --- | --- |
| #4085 | Diagnostic/design only | Explicitly excluded by #4096; keep as follow-up or close as intentionally deferred/dropped. |
| #4086 | Superseded | Replaced by #4096 routing convergence. |
| #4087 | Partially ported | Replacement evidence is #4111 plus `gt-12-capacity-admission`. |
| #4088 | Superseded | Replaced by #4096 routing convergence. |
| #4089 | Partially ported | Active-MR reuse/list/SLOT_OPEN semantics moved into workstate/recovery gates; remaining startup fold is #4114. |
| #4092 | Superseded | Replaced by #4096 routing convergence. |

## Current Release Decision

No-go for gt v1.2.0 as of this review. Release blockers remain because:

1. #4080, #4081, and #4096 are still open and red or unstable.
2. #4110-#4114 are active successor PRs added to the milestone but not yet
   merged.
3. The capacity/admission and release-candidate/canary epics still have open
   child beads.
4. The MR target/source track is locally complete but still needs a final
   upstream PR or explicit deferral recorded before release cut.
5. Canary, release quality gates, RC install, and final cut evidence are still
   pending.

## Post-Implementation Review Passes

1. Membership disposition: #4110-#4114 were added to the milestone; #4073-#4079,
   #4080, #4081, and #4096 remain present.
2. Superseded-only disposition: excluded #4085-#4089/#4092 are documented with
   replacement paths and are not milestone blockers by themselves.
3. Owner disposition: every blocking source issue/PR maps to a canonical local
   epic owner or a pending final-PR decision.
4. Evidence disposition: red/clean status and open local child beads are recorded
   as release-decision evidence.
5. Scope disposition: documentation and GitHub milestone metadata only; no
   runtime code or release mechanics changed.
