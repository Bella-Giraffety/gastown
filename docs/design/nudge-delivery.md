# Nudge Delivery

## Summary

Gas Town has two competing goals for `gt nudge`:

- deliver quickly enough to be useful for coordination
- avoid corrupting human or agent input in a shared CLI session

Issue `#1216` documents the core constraint: the final hop still goes through the
agent's interactive input buffer. That buffer is shared with the overseer and is
not queryable, transactional, or safely clearable across runtimes.

## Current Design

The current implementation is split across four pieces:

- `internal/cmd/nudge.go`: chooses `wait-idle`, `queue`, or `immediate`
- `internal/cmd/nudge_poller.go`: drains queued nudges for runtimes without hook-based delivery
- `internal/nudge/queue.go`: persists queued nudges to `.runtime/nudge_queue/<session>/`
- `internal/tmux/tmux.go`: performs the final `send-keys` injection into the pane

Today we reduce failures with a layered approach:

1. `wait-idle` waits for a visible prompt before injecting.
2. `queue` persists the payload so a nudge is not lost if direct delivery is unsafe.
3. `sendKeysLiteralWithRetry` handles the cold-start race where tmux rejects input before the TUI is ready.
4. Per-session locking prevents concurrent nudges from interleaving.

This solves several operational problems, but it does not solve the shared-input problem.

## What Is Solved

- Startup race: retries cover tmux `send-keys` failures like `not in a mode`.
- Cross-process interleaving: flock plus in-process locks serialize delivery.
- Idle-path delivery: `wait-idle` avoids many avoidable interruptions.
- Queue durability: queued nudges survive until a later drain attempt.

## Unresolved Constraint

The final delivery path is still text injection into a live terminal input field.
That means the sender cannot reliably know any of the following before injecting:

- whether the overseer is typing
- whether the agent is mid-request but temporarily shows a prompt-like state
- whether the input field is empty
- what text must be preserved if we clear the field

The historical `clear/inject/verify` attempt in PR `#1212` explored forcing the
input field clear with `Ctrl-C`, injecting the nudge, then diffing terminal
captures to reconstruct what happened. That design found the collision reliably,
but it exposed a harder limit: clearing safely is not the same as restoring safely.

## Why Input Clearing Remains Unsafe

`Ctrl-C` is the only broadly portable way we found to clear arbitrary partial
input across supported runtimes. It is also unsafe in exactly the ways issue
`#1216` describes:

- it can interrupt in-flight agent work
- a second close `Ctrl-C` can exit Claude Code entirely
- after clearing, there is no universal, safe way to restore prior input when the field may already contain new text

The diff-based design can often tell us that a collision happened, but it cannot
guarantee a safe recovery path without another destructive clear step.

## Practical Consequence

Reliable payload preservation and reliable input preservation cannot both be
guaranteed while the payload itself is delivered through the shared input buffer.

The queue helps with durability, but queued nudges still need a final hop. If the
final hop is `send-keys` of the full payload, the shared-buffer collision window
still exists.

## Recommended Direction

The most promising direction is to stop sending the full payload through the
shared input buffer.

Instead:

1. persist the full nudge payload in the existing queue
2. inject only a tiny trigger, or rely on turn-boundary polling
3. have agents read queued nudges from a side channel at safe boundaries

That reduces the shared-buffer risk from "full message corruption" to "small
trigger noise" and keeps the real payload durable outside the terminal line editor.

The strongest long-term fixes would be one of:

- an agent-side API for side-channel nudge receipt
- a portable "clear input without interrupt" primitive
- a queryable input-buffer API
- standard stash/unstash semantics across supported runtimes

## Historical Note

PR `#1212` introduced `docs/design/nudge-delivery.md` for the earlier
clear/inject/verify design. That implementation was never merged, but the issue
continued to reference this path. This document records the current, in-tree
state instead of reviving the abandoned protocol as if it were still the plan.
