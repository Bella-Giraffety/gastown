# Cleanup Approval Triggers and Preserve-Weirdness Rules

This document defines when a change is no longer cleanup-safe and must either
get explicit human approval or default to preserving existing behavior.

## Core Rule

Cleanup may remove duplication, dead paths, or unnecessary indirection only when
the author can prove that externally relevant behavior is preserved.

If the safety argument depends on any of the following claims, the slice is not
cleanup-safe by default:

- "This weird behavior was probably accidental."
- "Nobody should rely on this ordering or fallback."
- "The new behavior is better even if something changes."
- "This difference is only visible in edge cases."
- "The old path is wrong, so normalizing it is still cleanup."

When a cleanup claim requires one of those arguments, stop and either preserve
the existing behavior exactly or defer the change for explicit approval.

## Human-Approval Triggers

The following cases require explicit human approval before merge. They are
behavior changes or behavior-risk changes, not cleanup-only work.

| Trigger | Why it is not cleanup-safe | Default action |
|---------|----------------------------|----------------|
| Routing or precedence changes | Changing lookup order, winner selection, or fallback precedence can redirect work even when all branches still "work" | Preserve current ordering; defer if the new order is intentional |
| Lifecycle timing shifts | Starting, retrying, timing out, handing off, cleaning up, or shutting down earlier/later can change visible outcomes and race behavior | Preserve timing/ordering; escalate if the shift is required |
| Runtime behavior differences | Changing outputs, errors, exit codes, prompts, emitted mail, side effects, operator-visible logs, or metrics changes the program contract | Keep the current surface unless the change is explicitly requested |
| Compatibility quirk removal | Removing tolerated legacy inputs, old env fallbacks, path quirks, or historical alias behavior can break existing operators and automation | Preserve the quirk and document it; defer cleanup of the quirk itself |
| Authority-path changes | Reassigning which component decides, validates, routes, or closes state changes system control flow even if final results often match | Treat as a design change, not cleanup |
| Invariant or proof-mode changes inside one slice | A PR that both changes the rule and claims to prove it collapses the review boundary | Split the work or get explicit approval |
| Uncertain duplicate-collapse | If two paths look equivalent but differ in sequencing, logging, retries, cleanup, or error shaping, they are not proven duplicates | Preserve both until equivalence is demonstrated |
| Uncertain dead-code claims | Code is not dead if reachability depends on runtime config, hooks, environment, external tools, malformed input, or recovery paths that were not exhaustively checked | Preserve it or add stronger proof |
| Cross-boundary contract changes | Changes that alter assumptions between CLI, daemon, witness, mayor, refinery, plugins, hooks, or shell scripts can move breakage to another subsystem | Treat as contract work, not cleanup |
| Operator-surprise arguments | "Users will prefer this," "reviewers expect this," or similar taste-based reasoning is approval-seeking, not proof of preservation | Escalate for human decision |

## Preserve-Weirdness Exceptions

Some behavior is strange but still part of the current contract. Preserve it
instead of normalizing it away when any of these are true:

- The weirdness is externally visible and existing tests, docs, or issue history
  show that someone may rely on it.
- The weirdness is part of precedence, fallback, lifecycle sequencing, or
  recovery behavior.
- The weirdness is only exercised in error paths, startup, shutdown, or other
  low-frequency paths where breakage is hard to detect before release.
- The weirdness exists to preserve compatibility with older commands, files,
  environment variables, or partial migrations.
- The cleanup author cannot prove that all call sites and runtime entry points
  observe identical behavior after normalization.

In those cases, the cleanup-safe move is:

1. Keep the behavior.
2. Add characterization coverage or a brief note if that makes the preserved
   behavior easier to see.
3. File a separate follow-up for intentional behavior change if the weirdness
   should eventually be removed.

## Defer-by-Default Cases

Default to preserve or defer when certainty is insufficient, especially for:

- precedence ladders with more than one fallback source
- startup or shutdown sequencing
- retries, backoff, polling, or timeout behavior
- shell-facing output and exit status
- hook injection, mail flow, or agent lifecycle state transitions
- legacy compatibility shims and migration glue
- paths that are only covered by operational experience instead of tests

The stop rule is simple: if the reviewer must trust intent instead of proof,
the slice is not cleanup-only.

## Required Escalation Note

When a slice hits an approval trigger, the author should say which trigger fired
and why the change was preserved or deferred. The minimum note is:

```text
Not cleanup-safe: <trigger>
Reason: <what behavior may change>
Action: preserved existing behavior | deferred for explicit approval
```

## Review Outcome

For cleanup-only PRs, approval should be blocked when any trigger above is
present without explicit human sign-off. "Looks equivalent" is not enough when
precedence, lifecycle, compatibility, or operator-visible behavior is in play.
