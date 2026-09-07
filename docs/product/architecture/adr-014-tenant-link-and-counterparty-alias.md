# ADR-014: TenantLink and CounterpartyAlias are new aggregates; pair
authorization is enforced before Post is ever called, not inside it

**Status**: Accepted, 2026-09-07
**Owner**: nw-ddd-architect (DESIGN wave, `inter-tenant-transfer`)

## Context

`inter-tenant-transfer` builds J8/J9/J10 (`docs/product/jobs.yaml`), the
first feature to move value between two tenants' own namespaces. DISCUSS
locked the constraint that made this feature's domain modelling
non-trivial: every leg of a transfer is strictly intra-tenant (D6,
`internal/domain/post.go:42-53`, I8) — "never one `Post` call touching two
tenants" — and locked a routine-trust authorization model (D9, standing
until revoked) plus a tenant-scoped addressing scheme (D11,
`counterparty_alias`, never a raw tenant id). Four DESIGN pre-requisites
were named and handed to this architect:

1. Aggregate shape for the tenant-pair authorization and the alias
   (feature-delta.md § Pre-requisites #1).
2. Whether a new invariant number is warranted for "a transfer's legs may
   only post between tenants with an active link," and where it is enforced
   (§ Pre-requisites #4).
3. New `ViolationKind` members and which decision site owns each
   (§ Pre-requisites #3).
4. Compensating-transaction mechanics under D7's append-only discipline
   (§ Pre-requisites #6).

Brownfield state, verified directly against `internal/domain/post.go`: I8's
refusal (lines 42-53) checks that both touched account snapshots share the
command's own `tenant_id`; nothing in this feature's slices asks that check
to change, weaken, or be duplicated. `adr-011-tenant-partition-not-context.md`
is the direct precedent for every judgment below — Tenant's own
aggregate-vs-value-object reasoning, its deliberately minimal field set, and
its rejection of child-entity ownership all transplant here with the same
force.

## Decision

**1. Two new aggregate roots: TenantLink and CounterpartyAlias.** Neither is
a value object (both carry identity that persists across a lifecycle — grant
then revoke; register then repeatedly resolve), neither is a child of
Tenant or of each other (their true invariants — active-uniqueness per pair,
and alias-name uniqueness per owning-tenant namespace — share nothing and
would force unrelated registrations/revocations to serialize against each
other for zero benefit). Root-only, value-typed properties on both —
Vernon's rule 2 satisfied by inspection, same shape as Tenant.

**2. A new invariant, I11, is warranted and is enforced by construction, at
two separate pre-construction gates, mirroring I8/I9/I10's own pattern
rather than I3's deliberately-unenforced one.** I11: "At most one active
TenantLink exists per unordered tenant pair at any time; and a cross-tenant
transfer may only be initiated between a pair currently holding an active
link." First gate: `AuthorizeTenantPair`'s own uniqueness-of-active check
(structurally identical to I10's tenant-name check, one collection over).
Second gate, in two enforcement points: `RegisterCounterpartyAlias` refuses
`tenant_link_not_found` against a non-active link at registration time, and
`ResolveCounterparty` refuses `counterparty_not_found` against a since-
revoked link at send time (deliberately the same kind as "alias never
existed," not a distinguishing kind — see point 4).

I11 is **not** enforced inside `Post` itself, and this is not a gap: `Post`
never sees two tenants in one call in the first place (leg 1 and leg 3 are
each purely intra-tenant; leg 2 is the internal platform-ledger movement,
intra-tenant with respect to the platform's own account scope). There is no
place inside `Post`'s existing signature where "the other tenant" is even
visible for I11 to check. I11 lives one layer up, gating whether the
coordinator may address the other tenant's account *at all* — which is
exactly what alias resolution already is the choke point for.

**3. `Transfer` is not a domain aggregate.** Modelled instead as
coordinator/application state (D8's own "per-transfer coordinator"
vocabulary), for three reasons: no true invariant lives there (each leg is
already fully decided the instant its own `Post` call succeeds); modelling
it as a domain aggregate would force a two-tenant consistency boundary of
exactly the shape I8 exists to forbid; and DISCUSS already named this a
saga/coordinator, not an aggregate, before DESIGN began. Consequence:
`transfer_not_found` (US-5) is not a `domain.ViolationKind` member — there
is no domain aggregate for it to be a violation of.

**4. Three new `ViolationKind` members, one candidate explicitly rejected as
new-in-name-only:**
- `tenant_link_already_exists` (409) — new, domain core, I11 first gate.
- `tenant_link_not_found` (404) — new, domain core, I11 second gate, first
  enforcement point (registration).
- `counterparty_not_found` (404) — **reused** existing wire-facing name,
  newly load-bearing for a second refusal reason (I11 second gate, second
  enforcement point, at resolution). Deliberately collapses "alias never
  existed" and "alias's link died" into one shape — the same info-leak
  discipline ADR-011 already established by reusing `account_not_found`
  rather than inventing `cross_tenant_forbidden`.
- `transfer_not_found` — explicitly **not** added to the sealed taxonomy
  (point 3, above).

**5. Reversal is a specific use of `Post`, not a new domain operation.** A
compensating leg is an ordinary `TransferCommand` with the movement
direction mirrored, run through the unmodified `Post` function — the same
I1/I4 checks apply because a compensating movement is not a different kind
of fact than an original one. This satisfies D7 (append-only) by
construction, since `Post` never edits or deletes an `Entry`. The guarantee
against double-reversal is I7's own idempotency principle
("the same request applied twice changes state once"), re-applied to
coordinator state keyed by `(transfer_id, leg_number, direction)` instead of
a caller-supplied key — not a new invariant, and not this architect's layer
to implement (coordinator state persistence is `nw-solution-architect`'s,
feature-delta.md § Pre-requisites #2).

## Consequences

- `TenantLink` and `CounterpartyAlias` join Tenant, Account, and Transaction
  as the Ledger context's aggregates. No second bounded context — the
  language-divergence check found none, and the one candidate that could
  have forced a split (`Transfer`-as-aggregate) is rejected.
- The sealed `ViolationKind` taxonomy (DDD-12) grows by exactly three
  members, not four — `transfer_not_found` stays an application-layer
  concern, so the `exhaustive` linter's domain-core surface obligation
  grows by three, and the wire-mapping surface (DDD-17) by the same three
  plus whatever `nw-solution-architect` wires for `transfer_not_found`
  through its own (non-taxonomy) mechanism.
- `Post`'s signature and I8's check (`post.go:42-53`) are **unchanged** —
  this feature composes with I8 rather than touching it, exactly as D6
  locked.
- Coordinator state persistence shape, the double-reversal guard's physical
  mechanism, and the `transfer_not_found`/US-5 authorization-boundary
  mechanism are explicitly not decided here — `nw-solution-architect`'s, per
  feature-delta.md § Pre-requisites #2 and #5.

## Rejected alternatives

- **CounterpartyAlias as a child entity of TenantLink** — rejected; would
  make every alias registration lock and reload the whole link record,
  serializing unrelated registrations and revocation against each other for
  an invariant none of them share (God Aggregate smell — unbounded child
  collection under one root).
- **CounterpartyAlias or TenantLink as a child entity of Tenant** — rejected
  for the same reason ADR-011 already rejected "Tenant owning Accounts as
  child entities": Tenant is deliberately kept to `{tenant_id, name,
  credential}` so posting never becomes a three-aggregate-type unit of
  work; growing it with an alias or link collection repeats that mistake.
- **A `Transfer` aggregate owning its three legs** — rejected (point 3,
  above); would force a two-tenant consistency boundary I8 exists to
  forbid, decide nothing Post doesn't already decide, and contradict D8's
  own coordinator/saga framing.
- **I11 enforced inside `Post`** — rejected; `Post` has no visibility into
  "the other tenant" in any single call (leg 1/3 are purely intra-tenant,
  leg 2 is intra-platform), so there is no correct place inside it to check
  a cross-tenant pair authorization. The check belongs at the one place
  that *does* see both tenants: alias resolution, ahead of any `Post` call.
- **A distinct `Reverse` domain function** — rejected; would duplicate
  I1/I4's own checking logic for a movement that is not a different kind of
  fact than any other movement `Post` already knows how to validate.
- **A new invariant number for the double-reversal guarantee** — rejected;
  it is I7's own principle, one layer up, not a new domain rule.

## References

- `docs/product/architecture/brief.md` § Domain Model / "Inter-tenant
  transfer (confirmed 2026-09-07)" — full bounded-change contracts for
  TenantLink and CounterpartyAlias.
- `docs/feature/inter-tenant-transfer/feature-delta.md` § Pre-requisites #1,
  #3, #4, #6; § Locked decisions D6-D11.
- `docs/feature/inter-tenant-transfer/slices/slice-01-authorize-a-tenant-pair.md`,
  `slice-02-send-a-transfer-to-a-named-counterparty.md`,
  `slice-04-reverse-after-retry-budget-exhausted.md`.
- `adr-011-tenant-partition-not-context.md`, `adr-008-refusal-taxonomy-boundary.md`,
  `adr-009-unavailability-is-not-a-refusal.md` — precedent this decision
  extends.
