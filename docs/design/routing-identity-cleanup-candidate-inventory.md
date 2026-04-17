# Routing And Identity Cleanup Candidate Inventory

This file is evidence-only. It inventories cleanup candidates in routing,
address normalization, identity attribution, and fallback authority paths without
proposing a combined cleanup branch.

Safety classes used here are provisional:

- `low`: local cleanup with a small semantic-equivalence proof surface
- `medium`: reviewable in one subsystem, but needs call-site inventory or targeted tests
- `high`: touches lifecycle, compatibility, operator-visible behavior, or multi-package fallbacks
- `defer`: not cleanup-safe until a separate migration or approval path exists

Preferred PR classes map to the cleanup contracts in
`cleanup-pr-shape.md`, `cleanup-proof-standards.md`, and
`cleanup-approval-triggers.md`.

## Candidates

| Candidate | Subsystem tag | Cleanup class | Provisional safety class | Preferred PR class | Proof notes needed | Evidence |
| --- | --- | --- | --- | --- | --- | --- |
| Unify or explicitly preserve the split address parsers in `internal/mail` and `internal/session`. Today they both parse agent identity, but they do not own exactly the same contract for short forms, role-bearing paths, or town singletons. | `identity/address-parsing` | duplicate-collapse | high | call-site inventory plus semantic-equivalence PR | Inventory every caller of `mail.AddressToIdentity`, `mail.identityToAddress`, and `session.ParseAddress`; prove preserved handling for `rig/name`, `rig/crew/name`, `rig/polecats/name`, `mayor/`, `deacon/`, and rig-scoped town aliases before collapsing either parser into the other. | `internal/mail/types.go:542-622`, `internal/mail/resolve.go:127-197`, `internal/session/identity.go:31-83`, `internal/session/identity_test.go:398-460` |
| Remove the short-address and slash aliases only after a full producer-consumer migration. The current release still accepts both explicit role paths and shorthand identities, and still normalizes `mayor`/`mayor/` and rig-scoped town aliases into one mailbox identity. | `mail/address-aliases` | stale-compatibility removal | high | dedicated stale-compat PR | Inventory all producers and stored consumers of `mayor/`, `deacon/`, `rig/name`, `rig/mayor`, and explicit `rig/polecats/name` addresses; prove mailbox lookup, recipient validation, and UI formatting stay stable when alias normalization is removed. | `internal/mail/types.go:545-609`, `internal/mail/types_test.go:9-78`, `internal/mail/router_test.go:1253-1270`, `internal/web/fetcher.go:1125-1146`, `internal/web/validate_test.go:86-116` |
| Remove the cwd-based sender fallback only after `GT_ROLE` and `.gt-agent` coverage is proven universal for agent-originated commands. The current sender authority ladder is env first, then agent metadata, then path parsing, then overseer. | `mail/sender-detection` | stale-compatibility removal | high | dedicated stale-compat PR | Inventory every command path that calls `detectSender`, including handoff, escalate, and manual debugging flows; prove agent sessions always provide `GT_ROLE` or `.gt-agent`, and preserve current overseer/manual behavior before deleting cwd parsing. | `internal/cmd/mail_identity.go:79-167`, `internal/cmd/mail_identity.go:235-290` |
| Align worker attribution env vars only after an explicit migration plan. `BD_ACTOR` uses the slash-path identity, while worker `GIT_AUTHOR_NAME` and `BEADS_AGENT_NAME` still carry shorter historical forms in code and tests even though docs describe a single identity contract. | `identity/env-attribution` | authority-path cleanup | defer | preserve-or-migrate first, cleanup second | Inventory every reader and writer of `BD_ACTOR`, `GIT_AUTHOR_NAME`, and `BEADS_AGENT_NAME`, plus audit queries, docs, and tests; prove git history, mail routing, beads attribution, and startup env repair preserve the same identity semantics if the worker fields are unified. | `internal/config/env.go:82-159`, `internal/config/env_test.go:35-57`, `internal/config/env_test.go:71-76`, `internal/polecat/session_manager_test.go:252-267`, `docs/concepts/identity.md:18-33`, `docs/concepts/identity.md:42-75`, `docs/concepts/identity.md:94-119`, `docs/reference.md:279-295` |
| Retire workspace-directory recipient validation fallback only after agent bead coverage is proven universal. Current mail routing still uses the filesystem as a secondary truth source when bead-backed identity lookup is incomplete or unavailable. | `mail/recipient-validation` | stale-compatibility removal | high | call-site inventory PR | Inventory every valid recipient class that can exist before or without an agent bead, especially fresh polecats, dogs, and recovery flows; prove beads-backed validation alone can still reject typos like `testrig/mayor` while accepting all supported recipients. | `internal/mail/resolve.go:161-197`, `internal/mail/router_test.go:1250-1269` |
| Remove the `routing.mode=explicit` safety net only after beads auto-routing is re-proven in the supported release matrix. The current check exists because upstream auto mode can route town work into `~/.beads-planning` based on remote shape. | `beads/routing-mode` | stale-compatibility removal | defer | preserve-or-migrate first, cleanup second | Prove the supported beads version no longer misroutes under HTTPS and file remotes, inventory bootstrap and doctor fix paths that depend on explicit mode, and run operational checks showing local mail and issue writes stay in town and rig stores without `BEADS_DIR` overrides. | `internal/doctor/routing_mode_check.go:11-147` |

## Slicing Notes

- The cleanest later slice is probably sender-detection fallback removal, but only
  if a call-site inventory shows agent commands no longer rely on cwd parsing.
- The highest-risk group is the identity-format set: short mail identities,
  worker git attribution, and dual parser ownership all touch the routing and
  authority invariants called out in `cleanup-behavior-invariants.md`.
- The routing-mode and workspace-recipient fallbacks are historical safety nets.
  They should be treated as preserve-or-defer items until a replacement proof is
  stronger than the current operational hedge.
