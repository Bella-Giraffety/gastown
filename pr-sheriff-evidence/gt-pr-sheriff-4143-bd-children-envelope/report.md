# PR Sheriff Report

Subject: `gastownhall/gastown` PR #4143, `fix(daemon): parse bd show --children envelope so molecule steps close (#4142)`

URL: https://github.com/gastownhall/gastown/pull/4143

Mode: individual_review / merge_gate_report

Author: `dickeyf` / Francois Dickey, contributor tier `unknown`

Base/Head: `main@241a72c642975b43f094c8096b3ed1430dd4adc9` -> `dickeyf/gastown:fix/dog-molecule-children-parse@14adfda9cbeb044e25227359369672e68ebc8483`

Current upstream/main: `09cf8ce37558316ef7b09d23a2ce4758527a3523`

Labels: status=`reviewing`, priority=`p2`, kind=`bug`

Action mode: report-only defer, no Sheriff code change

Research legs: 15/15

Pre-decision reviews: 5/5 approve

Post-implementation reviews: not applicable

Cleanup-first: `acceptable_minimal`

Final verdict: `defer_human_review`

Merge path allowed: `false`

## Blocking Gates

- `merge_status`: PR is `status/reviewing`, not `status/review-approved` or `status/merge-ready`.
- `blocker_scan`: no formal reviews/approvals exist; PR is stale behind current upstream/main; merge readiness remains unresolved.
- `focused_verification`: PR-head checks are only label-management workflows; no substantive Go build/test or focused daemon verification is attached to head `14adfda9`.

## Evidence Summary

- Diff is narrow: `+26/-0` in `internal/daemon/dog_molecule.go` and `internal/daemon/dog_molecule_test.go`.
- Local reproduction of the relevant Beads output shape: `bd show gt-we2z --children --json` returned a parent-ID object with top-level `schema_version`.
- The PR intent is valid and cleanup-first acceptable: it extends the existing `parseChildrenJSON` path rather than adding retries, shims, or alternate commands.
- Security review found no auth, secrets, credentials, dependency, CI permission, or shell-injection risk.
- Attribution must be preserved if this is carried forward: PR author is `dickeyf` / Francois Dickey; commit author is `mayor <francois@r730.local>` and includes `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.

## Required Next Actions

- Do not merge PR #4143 as-is while status, approval, and verification gates are missing.
- If carried forward by Sheriff or maintainer, preserve attribution to `dickeyf` / Francois Dickey and cite PR #4143.
- Refresh, fix up, or replace against current upstream/main before a final merge-path decision.
- Run focused daemon verification on the final PR/replacement head, at minimum `go test ./internal/daemon -run TestParseChildrenJSON -count=1` and `go test ./internal/daemon -count=1`, then capture output tied to the final head SHA.
- Promote labels only after review approval and verification evidence exist, then rerun `gt-pr-sheriff-check --merge-gate`.

## Checker Results

Non-merge validation passed:

```text
Command: gt-pr-sheriff-check --evidence pr-sheriff-evidence/gt-pr-sheriff-4143-bd-children-envelope/evidence.json
Exit code: 0
PR Sheriff: PASS
merge_path_allowed: false
verdict: defer_human_review
gates: 5 pass, 6 not_applicable, 0 waived, 3 fail
```

Merge-gate validation blocked:

```text
Command: gt-pr-sheriff-check --evidence pr-sheriff-evidence/gt-pr-sheriff-4143-bd-children-envelope/evidence.json --merge-gate
Exit code: 1
PR Sheriff: BLOCK
merge_path_allowed: false
verdict: defer_human_review
gates: 4 pass, 6 not_applicable, 0 waived, 4 fail
blocking_gates:
- merge_status: status reviewing is not merge-ready; merge-ready status is contradicted by unresolved blocker evidence
- blocker_scan: unresolved blockers found
- focused_verification: verification requires focused_checks, ci_checks, or a not_applicable reason; verification.ci_checks missing configured baseline checks: Integration Tests, Lint, Reject go.mod replace directives, Reject issues.jsonl, Test, Windows Smoke Test; no passing or explicitly not-applicable verification evidence found
- final_verdict: --merge-gate requires final.verdict merge_as_is or merge_replacement
```

Evidence files:

- `pr-sheriff-evidence/gt-pr-sheriff-4143-bd-children-envelope/evidence.json`
- `pr-sheriff-evidence/gt-pr-sheriff-4143-bd-children-envelope/non-merge-check.txt`
- `pr-sheriff-evidence/gt-pr-sheriff-4143-bd-children-envelope/merge-gate-check.txt`
