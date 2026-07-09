# PR Sheriff Evidence Recovery: PR #4447

Subject: `gastownhall/gastown` original PR #4061, replacement target PR #4447

Final verdict: `merge_replacement`

Merge path allowed: `true`

Target head: `0bcc9c507ba243d5c697c5cb9634e2c614a45643`

Evidence file: `pr-sheriff-evidence/gt-mll4-pr-4447-recovery/evidence.json`

Checker command:

```bash
gt-pr-sheriff-check --evidence "pr-sheriff-evidence/gt-mll4-pr-4447-recovery/evidence.json" --merge-gate
```

Checker result:

```text
PR Sheriff: PASS
merge_path_allowed: true
verdict: merge_replacement
gates: 12 pass, 2 not_applicable, 0 waived, 0 fail
```

Gate summary:

- 15 independent research legs completed.
- 5 independent pre-decision reviews approved `gt-mll4-action-merge-replacement`.
- 5 independent post-implementation reviews approved target head `0bcc9c507ba243d5c697c5cb9634e2c614a45643`.
- Labels are complete: `status/review-approved`, `priority/p2`, `kind/bug`.
- GitHub CI is green for the target head.
- Blocker scan found no unresolved blockers.
- Cleanup-first assessment is `convergent`.
- Replacement flow preserves #4061 attribution through PR body/reference and `Co-authored-by: Emmanuel Sciara <emmanuel.sciara@gmail.com>`.

Required follow-up after merge:

- After PR #4447 lands, close or supersede original PR #4061 with attribution preserved.
