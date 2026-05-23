# Release v1.2 PR Disposition Map

This map covers the integration-target PRs called out by `gt-12-superseded-pr-map`: #4086, #4088, #4092, #4087, #4089, and diagnostic PR #4085.

Current rule: do not close an integration-target PR until the replacement listed below has landed, or until a maintainer explicitly accepts the documented deferral/drop rationale.

## Pre-Implementation Review Evidence

1. PR metadata pass: #4086, #4088, and #4092 all target `integration/test-beaddolt-hardenning` and overlap on routing/sling safeguards. #4096 is the clean `main`-target rebuild and explicitly supersedes #4092, #4086, and #4088.
2. Routing bead pass: `gt-pr-main-routing-rebuild` and `gt-12-land-4096-routing` record the replacement path, post-review fixes, aggregation-branch delivery, and closure rule for #4086/#4088/#4092.
3. Diagnostic split pass: #4085 is not included in #4096. Its doctor/design route diagnostics remain a separate diagnostic follow-up unless maintainers intentionally drop them.
4. Capacity pass: #4087 is replaced by focused capacity/admission gate work, especially `gt-12-fold-4087-capacity`, `gt-12-rebase-4081-admission`, and PR #4111 into `integration/gt-1-2-capacity-and-admission-gate-admission`. The main-target tracker remains `hq-gtarch-pr-main-4087-capacity-fold`.
5. Reuse/startup pass: #4089 is only partially extracted. Workstate/reuse/list/SLOT_OPEN semantics are covered by #4080 and the `gt-12-polecat-workstate` leaves; tmux/session-startup hardening remains tracked by `hq-gtarch-pr-main-4089-reuse-startup-fold`.

## Disposition Table

| PR | Area | Status | Replacement | Closure Gate |
| --- | --- | --- | --- | --- |
| #4085 | Routing diagnostics/design | Deferred diagnostic | No main-target replacement yet. Source bead: `gt-rca-canon-routing-repair-design`. | Keep open unless maintainers explicitly defer/drop the doctor/design diagnostic layer. If dropped, close as intentionally deferred, not superseded by #4096. |
| #4086 | Rig add prefix route hijack | Superseded | #4096, plus `gt-12-land-4096-routing` under `gt-12-routing-identity`. | Close only after #4096 or the Mayor-packaged routing identity aggregate lands. |
| #4088 | Newly-created rig bead sling smoke | Superseded | #4096, plus `gt-12-land-4096-routing` and `gt-12-formula-identity-tests`. | Close only after #4096 or the Mayor-packaged routing identity aggregate lands. |
| #4092 | Collapsed routing convergence integration PR | Superseded | #4096 clean `main`-target rebuild, delivered through `gt-12-land-4096-routing`. | Close only after #4096 or the Mayor-packaged routing identity aggregate lands. |
| #4087 | Recovery-slot capacity accounting | Partially ported / replacement in progress | `gt-12-fold-4087-capacity`, `gt-12-rebase-4081-admission`, PR #4111, and tracker `hq-gtarch-pr-main-4087-capacity-fold`. | Close after capacity/admission replacement lands or the hq tracker records that #4087 behavior is fully covered. |
| #4089 | Polecat reuse and session startup hardening | Partially ported | Reuse/workstate semantics: #4080, `gt-12-route-consumers-workstate`, `gt-12-action-leases`, `gt-12-stale-cleanup-active-mr`, and `gt-12-live-polecat-fixtures`. Startup hardening: `hq-gtarch-pr-main-4089-reuse-startup-fold`. | Do not close as fully superseded until the hq tracker either rebuilds the tmux/session-startup slice or records why it is obsolete. |

## Maintainer Closure Text

Use these comments after the corresponding closure gate is satisfied.

### #4086

```markdown
Closing as superseded by the clean main-target routing replacement. #4096 / the routing identity aggregate includes the useful #4086 prefix ownership guard, route mutation serialization, AddRig rollback coverage, and same-rig prefix invariant tests without carrying the integration/test-beaddolt-hardenning branch history.

Replacement: #4096, tracked by gt-12-land-4096-routing.
```

### #4088

```markdown
Closing as superseded by the clean main-target routing replacement. The useful fresh rig-bead sling smoke coverage from #4088 was rebuilt in #4096 and supplemented by the routing identity gate tests.

Replacement: #4096, tracked by gt-12-land-4096-routing and gt-12-formula-identity-tests.
```

### #4092

```markdown
Closing as superseded by the clean main-target routing replacement. #4092 was the integration-target collapsed routing PR; #4096 rebuilds the routing safeguards on the main-target path and avoids retargeting integration/test-beaddolt-hardenning history.

Replacement: #4096, tracked by gt-12-land-4096-routing.
```

### #4087

```markdown
Closing as superseded/ported after the capacity-admission replacement landed. The recovery-slot capacity behavior from #4087 is covered by the focused capacity/admission gate work rather than merging the integration-target PR directly.

Replacement: gt-12-fold-4087-capacity / gt-12-rebase-4081-admission, tracked by hq-gtarch-pr-main-4087-capacity-fold.
```

### #4089

```markdown
Closing as partially extracted after the remaining startup-hardening decision was resolved. The reuse/workstate portions are covered by the canonical polecat workstate and recovery false-positive gates; any tmux/session-startup hardening was either rebuilt separately or explicitly deemed obsolete.

Replacement: #4080 plus gt-12-route-consumers-workstate, gt-12-action-leases, gt-12-stale-cleanup-active-mr, and hq-gtarch-pr-main-4089-reuse-startup-fold.
```

### #4085

```markdown
Closing as intentionally deferred, not as superseded by #4096. #4096 covered the release-blocking routing corruption class; #4085's doctor/design route diagnostic layer remains diagnostic-only and was not included in the release replacement. Reopen or create a fresh main-target PR if maintainers want that diagnostic layer for a later release.

Source bead: gt-rca-canon-routing-repair-design.
```

## Open Blockers And Deferrals

- #4085 has no replacement PR. Treat it as deferred/dropped only by explicit maintainer decision.
- #4087 closure still depends on the capacity/admission replacement being accepted or `hq-gtarch-pr-main-4087-capacity-fold` recording full coverage.
- #4089 closure still depends on `hq-gtarch-pr-main-4089-reuse-startup-fold` resolving whether the tmux/session-startup hardening is rebuilt or obsolete.
- #4086, #4088, and #4092 are safe to close only after #4096 or its routing identity aggregate lands.

## Post-Implementation Review Evidence

1. Coverage pass: every requested PR has exactly one row and matching closure text. #4085 is included as a diagnostic PR rather than being hidden under the routing supersession cluster.
2. Replacement accuracy pass: #4086/#4088/#4092 point to #4096 and `gt-12-land-4096-routing`; #4087 points to the capacity/admission gate and hq tracker; #4089 points to both extracted workstate leaves and the remaining startup tracker.
3. Closure safety pass: the map explicitly blocks closing superseded PRs before the replacement lands, and separately blocks #4087/#4089 until their trackers record full coverage or accepted deferral.
4. Maintainer actionability pass: closure comments are copy-ready and state whether the closure is superseded, partially extracted, or intentionally deferred.
5. Scope pass: the change is documentation-only and does not modify release code, issue state, or GitHub PR state. Validation is the documented metadata/bead review plus markdown self-review.
