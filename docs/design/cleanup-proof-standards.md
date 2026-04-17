# Cleanup Proof Forms and Evidence Standards

This document defines what counts as sufficient proof for a
"no-behavior-change" cleanup slice in gastown. The goal is to make cleanup
review evidence-based instead of taste-based.

## Core Rule

Every cleanup-only slice must state:

- the specific cleanup claim being made
- the subsystem boundary being touched
- the primary proof form used
- the concrete evidence attached to that proof
- any residual uncertainty that forces preserve/defer instead of merge

A cleanup slice is not reviewable if it relies on an unstated proof mode or on
reviewer intuition to fill gaps.

## Allowed Proof Forms

The following proof forms are allowed for cleanup-only work.

### 1. Characterization Tests

Use this when the behavior being preserved can be exercised directly and the
test can pin the relevant externally visible outputs, errors, side effects, or
operator-visible state.

This is sufficient when:

- the changed path is reachable in a deterministic test harness
- the asserted observations cover the behavior that could regress
- the test distinguishes the kept behavior from plausible wrong behavior

Minimum evidence:

- the exact tests added or updated
- a short statement of what behavior those tests characterize
- why the assertions cover the changed cleanup claim rather than adjacent code

Not sufficient when:

- the test only covers a happy path while the cleanup changes error or edge behavior
- the assertions are so broad that materially different behavior would still pass
- the test is unrelated smoke coverage and does not exercise the changed path

### 2. Existing-Test Coverage Rationale

Use this when the code change is narrow and the relevant behavior is already
covered by stable existing tests, so adding new tests would be redundant.

This is sufficient when:

- the author names the exact existing tests that exercise the changed path
- the diff does not expand the reachable behavior surface beyond what those tests cover
- the safety argument explains why the existing assertions would fail if the cleanup were wrong

Minimum evidence:

- named existing tests or suites
- a call-path explanation from the changed code to those tests
- a short rationale for why no new characterization test is needed

Not sufficient when:

- the argument is only "tests are already green"
- coverage is inferred from package-level ownership instead of named exercising tests
- the cleanup changes routing, timing, lifecycle, or side effects that the cited tests do not observe

### 3. Line-Level Semantic Equivalence Proof

Use this when the cleanup is local enough that behavior preservation can be
shown directly from the code: same inputs, same outputs, same side effects,
same errors, and same externally visible ordering.

This is sufficient when:

- the before/after control flow is small enough to inspect precisely
- the proof names the preserved invariants, not just the changed lines
- the argument accounts for nil/error behavior, side-effect ordering, and early returns where relevant

Minimum evidence:

- the exact before/after region being compared
- a short invariant list covering outputs, errors, side effects, and ordering
- an explanation of why removed or folded statements are redundant rather than merely similar

Not sufficient when:

- the argument is only "the code looks equivalent"
- only the happy path is compared
- the proof ignores side effects such as logging, file writes, hook dispatch, env reads, or state mutation

### 4. Dead-Path Proof

Use this when the claim is that code is unreachable, inert, or impossible to
select in the supported runtime model.

This is sufficient when:

- all entry paths into the code are enumerated and ruled out
- the argument covers direct calls, indirect dispatch, hooks, config selection, and lifecycle registration where relevant
- the proof shows the removed path does not participate in a supported authority path

Minimum evidence:

- the enumerated possible entry points
- the reason each entry point cannot occur
- the source of truth used for that claim, such as routing tables, registration sites, config gates, or hook attachment rules

Not sufficient when:

- the argument is only "ripgrep found no references"
- the path is dead only under one environment but still reachable in supported setups
- the proof ignores reflection, registries, dynamic command lookup, or config-driven selection that this subsystem uses

### 5. Call-Site Inventory Proof

Use this when the cleanup depends on proving that all consumers of an API,
helper, command, or path are known and compatible with the proposed collapse or
removal.

This is sufficient when:

- every live caller is listed
- each caller is shown to receive equivalent behavior after the change
- there are no unknown external callers within the supported boundary

Minimum evidence:

- a complete call-site inventory inside the claimed subsystem boundary
- treatment of each call site after the cleanup
- justification for why hidden callers are not possible or are outside the allowed cleanup scope

Not sufficient when:

- the inventory omits generated paths, registration sites, interface impls, or command wiring
- the proof assumes package-private visibility automatically means no behavior risk
- the author collapses call sites with different preconditions, error handling, or side-effect expectations

### 6. Operational Evidence

Use this as supporting proof when static reasoning is not enough, especially for
startup behavior, lifecycle sequencing, interactive flows, or environment-bound
effects.

This is sufficient when:

- the observed run directly exercises the changed behavior surface
- the command, environment, and expected observations are documented
- the evidence is paired with another proof form unless the behavior is only observable operationally

Minimum evidence:

- the exact command or scenario run
- the relevant environment assumptions
- the concrete observed outputs, logs, state changes, or absence of changes

Not sufficient when:

- the author says "I ran it and it looked fine"
- the run is ad hoc and undocumented
- the operational check is used to justify a broad semantic claim that was not actually observed

## Evidence Composition Rules

- A slice may use more than one proof form, but it must name one primary proof.
- Operational evidence alone is usually not enough for duplicate-collapse or
  dead-code-removal claims unless the behavior is inherently runtime-only.
- When a proof depends on assumptions about routing, lifecycle, dynamic lookup,
  or unsupported callers, those assumptions must be stated explicitly.
- If the proof cannot account for a known weirdness, compatibility quirk, or
  cross-boundary contract, the change is not cleanup-safe and must preserve or
  defer.

## Minimum Evidence by Claim Type

### Duplicate-Collapse Claims

"Duplicate collapse" means removing or merging two implementations, wrappers,
or paths because they are claimed to be behaviorally the same for all retained
callers.

Minimum required evidence:

- a complete call-site inventory for the duplicate paths being collapsed
- line-level semantic equivalence proof for the kept behavior or for the shared helper extracted from both paths
- explicit treatment of differences in errors, logs, side effects, ordering, and nil/empty handling
- either characterization tests or an existing-test coverage rationale for the behavior observed by callers

Additional requirements:

- If the two paths differ in any externally visible behavior, the change is not
  cleanup-only unless that difference is proven dead for all supported callers.
- If one duplicate is only "mostly" redundant and relies on undocumented
  quirks, preserve/defer and send it through the human-approval path.

Insufficient duplicate-collapse proof patterns:

- "the functions are almost identical"
- "both callers passed in manual testing"
- "one path is newer so it should replace the old one"
- a call-site list with no reasoning about caller-specific expectations

### Dead-Code Removal Claims

"Dead code" means code that cannot run, cannot be selected, or has no supported
effect in the current program.

Minimum required evidence:

- a dead-path proof that enumerates all possible entry points
- a call-site inventory for direct and indirect references where applicable
- explicit treatment of registries, hooks, config flags, command wiring, and runtime dispatch relevant to that path
- supporting characterization or operational evidence when the deadness depends on runtime selection rather than purely static reachability

Additional requirements:

- If the code is reachable only in unsupported or impossible states, the proof
  must say which states are unsupported or impossible and why.
- If the code might still be an operator escape hatch, compatibility shim, or
  recovery path, it is not dead-code-safe without explicit approval.

Insufficient dead-code proof patterns:

- "no one calls this anymore"
- "grep found no references"
- "it never fired in one local run"
- removing a fallback path without proving the selection logic can never choose it

## Explicitly Unacceptable Proof Patterns

These arguments do not satisfy cleanup review on their own:

- compile/build success with no behavior argument
- unrelated green tests with no path-specific rationale
- coverage percentages with no named observing tests
- "the diff is small"
- "this is just refactoring"
- reviewer intuition that the code "looks safe"
- reasoning that ignores errors, logs, side effects, ordering, or lifecycle hooks
- claims of deadness or duplication that do not account for dynamic dispatch or config-driven behavior

## Reviewer Decision Rule

Approve a cleanup-only slice only when the evidence bundle matches the claim.
If the author cannot show a sufficient proof form with the minimum evidence for
that claim type, the slice is either under-proven or not actually cleanup-only.
