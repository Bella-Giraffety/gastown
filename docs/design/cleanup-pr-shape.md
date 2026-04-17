# Cleanup-Only PR Shape Contract

> **Bead:** gt-djp.4.3
> **Date:** 2026-04-17
> **Author:** guzzle (gastown polecat)
> **Status:** Draft contract
> **Related:** gt-djp.4, gt-djp.4.5

## Goal

Make cleanup review objective at author time by forcing small, independent PRs.
The default slice is one cleanup PR that carries one safety argument across one
subsystem boundary. If the proof story changes, the PR splits.

## Default Rule: Independence First

Cleanup PRs are independent by default. Reviewers should assume each PR can be
understood, tested, approved, reverted, or dropped on its own.

Sequencing is exception-only. An author may stack or sequence cleanup PRs only
when independence is impossible because a prior PR establishes a prerequisite
subsystem contract, authority path, or invariant that the later PR depends on.
The dependency must be named explicitly in the PR summary.

## Author-Time Shape Rules

Every cleanup-only PR must satisfy all of these rules before review:

1. One PR, one safety argument.
   The PR proves one thing: for example semantic equivalence, dead-path removal,
   narrowed authority, or unused call-site elimination. If the reader must switch
   proof modes mid-review, split the PR.
2. One PR, one subsystem boundary.
   Keep the change inside a single reviewable boundary such as witness cleanup,
   refinery queue logic, mail routing, or one CLI surface. Crossing into another
   subsystem is a split trigger unless the second subsystem is purely mechanical
   fallout inside the same proof story.
3. Prefer one authority path or invariant per PR.
   A cleanup slice should remove, document, or prove a single authority path or a
   single invariant. If the PR changes multiple independent routes to behavior,
   reviewers lose the ability to validate the safety claim locally.

## Split Triggers

Authors must split the work into separate PRs when any of the following changes:

1. Subsystem contract changes.
   If one part of the work changes how callers interact with a subsystem and a
   later part removes internals under the new contract, those are separate PRs.
2. Proof mode changes.
   Do not mix line-by-line equivalence, characterization-test proof, dead-path
   proof, operational evidence, or call-site inventory proof in one PR unless one
   mode is strictly incidental support for the same claim.
3. Cleanup class changes.
   Renames/moves, dead-code deletion, authority narrowing, invariant enforcement,
   and lifecycle cleanup should not share a PR when they can stand alone.
4. Invariant changes.
   If the reviewer must adopt a new invariant, preserve a new weird edge case, or
   reason about a different safety boundary, start a new PR.

## What Counts as a Valid Sequencing Exception

Sequencing is allowed only when all of the following are true:

1. PR B cannot be reviewed honestly without a fact established by PR A.
2. PR A still stands on its own as a complete cleanup slice.
3. PR B names the dependency and explains why rebasing the prerequisite into the
   same PR would weaken review clarity.
4. Each PR keeps its own single safety argument after the split.

Good examples:

- PR A narrows all call sites to one witness cleanup entrypoint; PR B removes the
  now-unreachable alternate path.
- PR A documents and proves the preserved lifecycle invariant; PR B deletes code
  that only existed for the pre-invariant world.

Bad examples:

- One PR both rewrites call routing and removes unrelated dead code in a second
  subsystem because "the tests still pass."
- One PR mixes rename churn, behavior proof, and invariant changes so the reader
  has to reconstruct which claim each hunk is supporting.

## Merge-Blocking Author Checklist

Before sending a cleanup-only PR for review, the author should be able to answer
"yes" to all of these:

1. Does this PR make exactly one safety argument?
2. Is the touched code inside one subsystem boundary?
3. Is there only one authority path or invariant under review?
4. If this PR depends on another, did I explain why sequencing is required?
5. If proof mode, cleanup class, subsystem contract, or invariant changed, did I
   split the work instead of combining it?

Any "no" means the PR is oversized for this cleanup program and should be split
before review.
