# Cleanup-Only Reviewer Checklist

Use this checklist when a PR claims to be cleanup-only. It turns the cleanup
contract into a reusable review artifact for authors and reviewers.

This checklist depends on:

- [`cleanup-behavior-invariants.md`](cleanup-behavior-invariants.md)
- [`cleanup-proof-standards.md`](cleanup-proof-standards.md)
- [`cleanup-pr-shape.md`](cleanup-pr-shape.md)
- [`cleanup-approval-triggers.md`](cleanup-approval-triggers.md)

## Required PR Metadata

Cleanup-only PRs must name all of the following in the PR body before review:

- subsystem boundary
- preserved invariant
- primary proof form
- evidence bundle
- approval-trigger status

If any field is missing, the PR is under-specified and should not pass the
cleanup-only gate.

## Reviewer Decision Rule

Approve a cleanup-only PR only when every answer below is "yes."

1. Is the subsystem boundary named, specific, and limited to one reviewable
   boundary?
2. Does the PR name the preserved invariant explicitly instead of relying on
   "no behavior change" shorthand?
3. Is the primary proof form one of the allowed proof forms from
   [`cleanup-proof-standards.md`](cleanup-proof-standards.md)?
4. Does the evidence bundle contain concrete evidence for that proof form,
   rather than "tests are green" or another taste-based claim?
5. Does the diff still make exactly one safety argument inside one subsystem
   boundary?
6. If the PR depends on earlier cleanup work, does it name the sequencing
   exception and explain why independent review is impossible?
7. Does the PR avoid proof-mode changes, cleanup-class changes,
   cross-boundary contract changes, or invariant changes inside one slice?
8. Does the approval-trigger field say `none`, or explicitly name the trigger
   and show that the author preserved or deferred the risky behavior?

Any "no" means the PR is either under-proven, oversized, or not actually
cleanup-only.

## Merge-Blocking Outcomes

Block cleanup-only approval when any of the following is true:

- the PR body omits required cleanup metadata
- the preserved invariant is unnamed or mismatched to the touched subsystem
- the proof form is not allowed for the claim type
- the evidence bundle does not meet the minimum evidence for the named proof
- the PR mixes multiple safety arguments, subsystem boundaries, or proof modes
- an approval trigger is present without explicit human sign-off

Use the escalation note from
[`cleanup-approval-triggers.md`](cleanup-approval-triggers.md) whenever the PR
is not cleanup-safe:

```text
Not cleanup-safe: <trigger>
Reason: <what behavior may change>
Action: preserved existing behavior | deferred for explicit approval
```
