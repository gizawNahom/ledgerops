# ADR-016: `GET /transfers/{transfer_id}` needs a new, resource-aware
authorization middleware — not a reuse of `requireTenantKeyOrOperatorKey`

**Status**: Accepted, 2026-09-07
**Owner**: nw-solution-architect (DESIGN wave, `inter-tenant-transfer`,
application leg)

## Context

US-5 requires that `GET /transfers/{transfer_id}` be readable by either
tenant party to the transfer, or the platform-admin credential, and refused
— with the identical 404 shape a nonexistent `transfer_id` would produce —
for a third tenant's own otherwise-valid `tenant_key`. `multitenancy`
already built a dual-mode middleware, `requireTenantKeyOrOperatorKey`,
for `GET /accounts/{id}/entries`: it grants access to the `OperatorKey`
*or* any resolved tenant, and relies on the query itself narrowing to the
caller's own accounts downstream. That shape does not fit here: a transfer
names two *specific* tenants, and "any resolved tenant" is exactly the wrong
grant — a third tenant with a perfectly valid `tenant_key` must still be
refused. Which two tenants a given `transfer_id` belongs to is a
per-resource fact, not something any existing credential-resolution
primitive can decide on its own.

## Decision

**A new middleware, `requireTransferParty`, composed from the existing
identity-resolution primitives (`bearerToken`, `isOperatorKey`,
`hashBearerToken`, the same `TenantKeyResolver` fallback), not built from
scratch and not a parameterization of `requireTenantKeyOrOperatorKey`:**

1. Resolve the caller's identity using the identical two-step check
   `requireTenantKeyOrOperatorKey` already performs. An unidentifiable
   caller is refused `401 unidentified_caller` — unchanged wire shape.
2. Read `chi.URLParam(r, "transfer_id")` and call
   `TransferStateRepository.Get` exactly once, regardless of what step 1
   resolved. A nonexistent `transfer_id` is refused `404
   transfer_not_found` immediately, before any identity comparison runs.
3. If found: an `OperatorKey` caller is admitted unconditionally. A
   `tenant_key` caller is admitted only if its resolved `tenant_id` equals
   the row's `tenant_id` or `counterparty_tenant_id`; otherwise refused
   `404 transfer_not_found` — through the same refusal call, from the same
   code path, as step 2's nonexistent-id case.
4. On success, the already-loaded state is injected into context so the
   handler does not re-fetch it.

**This satisfies "no response distinguishes forbidden from nonexistent" by
construction**: both refusal branches return through one function, after
exactly one database read either way. There is no code path where a
forbidden read costs an extra query, branch, or distinguishable log line —
the property US-5 needs is a consequence of there being only one way
through the middleware to a 404, not a discipline applied after the fact.

**The forged-`counterparty_alias` scenario (US-5) needs no new mechanism.**
`ResolveCounterparty`'s existing snapshot lookup (ADR-014) is already scoped
to the caller's own `tenant_id`; a forging tenant's lookup of an alias
registered in a different tenant's namespace structurally finds nothing.
Confirmed here, not built here.

## Consequences

- A fourth authorization middleware joins `requireOperatorKey`,
  `requireTenantKey`, `requireTenantKeyOrOperatorKey` in
  `internal/adapters/http/`, sharing the same `bearerToken`/`isOperatorKey`
  primitives (DDD-22's own factoring, extended one middleware further).
- `GET /transfers/{transfer_id}` is the only route this middleware guards;
  no other existing route's authorization behavior changes.
- The `401 unidentified_caller` and `404 transfer_not_found` wire shapes are
  both reused, unchanged — no new refusal shape is introduced.

## Rejected alternatives

- **Parameterizing `requireTenantKeyOrOperatorKey` with a per-route "allowed
  tenant set" callback** — considered, because it would technically avoid a
  fourth middleware file. Rejected: the "allowed tenant set" is not knowable
  until *after* a database read keyed by the URL parameter, which
  `requireTenantKeyOrOperatorKey`'s existing signature and every other
  caller of it (a static admin-or-any-tenant check with no such read) has no
  reason to accommodate — bolting a resource-lookup callback onto a
  primitive whose entire value today is being read-free would give every
  existing caller an unused capability and make the common case harder to
  read, for the sake of saving one small file.
- **Checking party membership inside the handler instead of a middleware**
  — considered, since chi middleware technically has access to
  `chi.URLParam` for a matched route. Rejected in favor of a middleware
  anyway: keeping the authorization decision in the same architectural
  layer (middleware) as every other credential decision in this codebase is
  what keeps "where is authorization decided" answerable by one grep over
  `router.go`, rather than "usually middleware, except this one resource,
  where it's in the handler" — a special case with no compensating benefit.
- **Reusing `account_not_found`-style reasoning to justify a 403 for the
  third-tenant case, distinguishing it from a genuine 404** — rejected
  outright; this is precisely the information-leak US-5 exists to close,
  and ADR-011's own precedent (reusing `account_not_found` over inventing
  `cross_tenant_forbidden`) already establishes this project's posture
  against it.

## References

- `docs/product/architecture/brief.md` § Inter-tenant transfer — full
  middleware pseudocode, driving-ports table.
- `docs/feature/inter-tenant-transfer/slices/slice-05-trace-and-isolate-a-transfer.md`
  — the UAT this middleware exists to satisfy.
- `adr-012-tenant-credential-mechanism.md` — the `requireTenantKeyOrOperatorKey`
  precedent this ADR distinguishes itself from.
- `adr-014-tenant-link-and-counterparty-alias.md`,
  `adr-011-tenant-partition-not-context.md` — the info-leak discipline this
  decision extends.
