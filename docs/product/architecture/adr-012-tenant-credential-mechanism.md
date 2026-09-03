# ADR-012: Tenant credential mechanism — dual-mode auth middleware, composed not duplicated

**Status**: Accepted (DDD-22, DDD-25). DDD-23 (item 6 below) was originally
left open; resolved 2026-09-03 by direct user decision — see § Amendment
below. Not superseded: the amendment settles the one item this ADR itself
deferred, it does not overturn anything this ADR decided.
**Owner**: nw-solution-architect (DESIGN wave, `multitenancy`, application leg)

## Context

`multitenancy`'s DISCUSS wave (D8) provisionally recommended reusing
`OperatorKey` as the platform-admin credential and adding a new `tenant_key`
credential type scoped to exactly one tenant, but explicitly deferred the
verification mechanism and its relationship to the existing
`requireOperatorKey` middleware to DESIGN (`feature-delta.md` § Pre-requisites
#3). The domain leg (`nw-ddd-architect`, ADR-011) settled the domain-modelling
half of the adjacent questions — Tenant as a new aggregate root, I8 enforced
by construction, cross-tenant reads reusing `account_not_found` — and handed
the credential mechanism itself to this architect.

Verified directly against `internal/adapters/http/router.go`: today, exactly
one middleware (`requireOperatorKey`) is mounted on exactly one `chi.Router`
group containing every protected route (`POST /accounts`, `GET
/accounts/{id}`, `GET /accounts/{id}/entries`, `POST /transfers`, `GET
/health/trial-balance`, `GET /console/verdict`). DISCUSS's own Driving Ports
table (stated as "DESIGN confirmation, not a new decision") partitions routes
into an admin-only set and a tenant-key-only set, plus one dual-mode route
(`GET /accounts/{id}/entries`).

## Decision

**1. `requireOperatorKey` is reused unmodified, mounted on more routes
(DDD-22).** Its function body does not change. It is applied to `POST
/tenants` (new) in addition to its existing routes. This is the cleanest
possible reuse: zero new code, zero new risk to the console-facing routes'
byte-identical behavior.

**2. Two new middlewares are added, composed from an extracted primitive, not
copy-pasted (DDD-22):**
- `bearerToken(r) (string, bool)` — the header-parsing line already inside
  `requireOperatorKey`, factored out and shared by all three middlewares.
- `requireTenantKey(resolve ports.TenantKeyResolver)` — looks up the
  presented token's hash via the new `TenantKeyResolver` driven port; not
  found → `401 unidentified_caller` (identical wire shape); found → injects
  `tenant_id` into request context via a typed key.
- `requireTenantKeyOrOperatorKey(operatorKey string, resolve
  ports.TenantKeyResolver)` — tries the exact `OperatorKey` string comparison
  first (byte-identical fast path, no I/O), falls back to `TenantKeyResolver`,
  refuses only if both fail. This is what makes the `OperatorKey` branch of
  `GET /accounts/{id}/entries` provably byte-identical to today's behavior,
  not merely similar.

**3. `TenantScope` is a closed, two-constructor type
(`ScopedToTenant(id)` / `Unscoped()`), never a nullable string.** Used only
where "unscoped" is a legitimate domain answer (`EntriesFor`, `TrialBalance`,
`ComputedBalances` — per ADR-011's "platform-wide aggregate" position).
Write-path ports (`AccountRepository.*`) keep `tenant_id` as a plain required
`string` — there is no legitimate unscoped write, and the type system should
not be able to represent one.

**4. Credentials are generated via the existing `IDGenerator` port
(`crypto/rand`-backed `google/uuid` v4), not a new randomness source.**
`tenant_id = "tnt_" + uuid.NewString()`, `tenant_key = "tk_" + uuid.NewString()`.
Stored as `sha256(tenant_key)` via the `crypto/sha256` package already
imported by `internal/adapters/http/handlers.go`. The plaintext key is
returned exactly once, at provisioning, and never persisted or logged.

**5. Two new sealed `domain.ViolationKind` members are required:
`tenant_already_exists` (409) and `tenant_not_found` (404) (DDD-25).** These
are distinct from — and do not contradict — ADR-011's statement that the
taxonomy does not grow for cross-tenant *access* (that statement is scoped to
I8's reuse of `account_not_found`). These two are I10's own pair, symmetric
to `account_already_exists`/`account_not_found` at the Account level. Both
grow the two `exhaustive`-linted switch surfaces DDD-12 already obligates
(owner: DEVOPS).

**6. One decision was explicitly NOT made in this ADR's original acceptance
(DDD-23)**: whether `POST /accounts`, `POST /transfers`, and `GET
/accounts/{id}` also become dual-mode (`OperatorKey` implicitly resolving to
a seeded legacy tenant) to keep the already-shipped
`demo-01`/`demo-02`/`demo-03`/`chaos-01` Makefile targets byte-identical, or
whether those targets instead get re-pointed at a second seeded
tenant-scoped credential, keeping `OperatorKey` free of any tenant-write
authority. Both were fully specified and buildable with the primitives
above; the choice was a security-posture/product trade-off, not a technical
one. **Resolved 2026-09-03 — see § Amendment below.**

## Consequences

- Three middlewares now decide `unidentified_caller`; the wire shape
  (`401 {"error":"unidentified_caller"}`) is identical from all three, so the
  sealed set does not grow, only the number of decision sites.
- `Deps` (composition root wiring, `internal/adapters/http/router.go`) gains
  one new field, `TenantKeyResolver`, wired in `cmd/api/main.go` the same way
  `Clock`/`IDGenerator` are — a plain function value, no new abstraction.
- No new external dependency: `crypto/sha256`, `crypto/rand` (transitively,
  via the already-vendored `google/uuid`), and `chi`'s existing nested-group
  support are all already present.

## Rejected alternatives

- **Extending `requireOperatorKey` itself to also check tenant keys** —
  rejected. Not "the existing function has too many dependencies" (an invalid
  justification per this project's own reuse-analysis discipline); the
  reverse: `requireOperatorKey`'s entire value is being a zero-dependency
  closure over one static string, used by callers (`POST /tenants`, unscoped
  verdict/trial-balance) that never need a database-backed lookup. Growing it
  to perform one would violate single-responsibility for those callers to
  serve a mechanism (keyed identity resolution) that is fundamentally
  different from what it does today (static-secret comparison).
- **A three-tier role system (admin / tenant-admin / tenant-member) built now**
  — rejected as speculative; no story asks for tenant-internal roles, and
  D7/D9 (operator-driven-only provisioning, no self-service) removes the only
  plausible driver for one in this feature's scope.
- **bcrypt/argon2 for credential hashing** — rejected; these are
  high-entropy random bearer tokens (122 bits via `crypto/rand`), not
  low-entropy user passwords. A slow, memory-hard KDF defends against
  brute-forcing a *guessable* secret; it adds latency and a new dependency
  for no defensive value against an unguessable one. SHA-256, already
  imported in this package, is the correct tool.

## Amendment (2026-09-03) — DDD-23 resolved

Item 6 above left one decision open at acceptance. The user has since
decided it directly (relayed by the coordinator), closing this ADR's one
outstanding item. Additive record, not a rewrite of the analysis above:

**Decision: Option C.** `POST /accounts`, `POST /transfers`, and `GET
/accounts/{id}` stay `tenant_key`-only, exactly as originally scoped in this
ADR's Decision § 2 table — `requireTenantKey`, not
`requireTenantKeyOrOperatorKey`, gates all three. `OperatorKey` gains no
tenant-write authority. The already-shipped `demo-01`/`demo-02`/`demo-03`/
`chaos-01` Makefile targets are kept passing by seeding a second, fixed demo
tenant credential (e.g. `LEDGEROPS_DEMO_TENANT_KEY`, mirroring
`LEDGEROPS_OPERATOR_KEY`'s existing pattern) bound to the same
`tnt_legacy_seed` tenant `adr-013-multitenancy-migration-shape.md` already
creates for independent schema reasons, and re-pointing the `Makefile`'s
shared `AUTH` variable's value to it.

**Rationale (user's stated reasoning)**: preserves the admin/tenant-credential
boundary this ADR's Decision § 1–2 already draw — `OperatorKey` remains
admin actions plus unscoped reads only, never a tenant's own write authority
— consistent with I8's isolation ethos and this project's
correctness/auditability-ranked-first quality attributes. Accepts the cost
named in item 6: touching a DEVOPS-owned file (`Makefile`) and seeding one
more fixed dev secret, precedented by `demo-operator-key` already being one.

**Option A (rejected)**: would have reused the `requireTenantKeyOrOperatorKey`
primitive already built for `GET /accounts/{id}/entries` at zero
`Makefile`-editing cost, at the price of `OperatorKey` becoming a credential
capable of moving money inside one tenant's ledger — judged not worth the
convenience.

**Inheritance, stated explicitly (mirrors DDD-12's `exhaustive`-linter
obligation being handed to DEVOPS elsewhere in this project)**: this ADR's
and DESIGN's job ends at the decision above. Seeding the second credential
and editing `Makefile`'s `AUTH` value is a **DEVOPS/DELIVER implementation
task**, not built in this wave.

Full record: `docs/feature/multitenancy/feature-delta.md` § Wave: DESIGN /
Application Architecture (DDD-23 row); `docs/feature/multitenancy/design/wave-decisions.md`.

## References

- `docs/product/architecture/brief.md` § Multitenancy — full narrative,
  contract-shape table, credential-to-port mapping.
- `docs/feature/multitenancy/feature-delta.md` § Pre-requisites #3, § Wave:
  DESIGN / Application Architecture.
- `adr-011-tenant-partition-not-context.md` — the domain-modelling decisions
  this ADR's wire mapping and credential mechanism build on.
- `adr-008-refusal-taxonomy-boundary.md` — the DDD-17 taxonomy this ADR
  extends with two new members.
- `Makefile` — the concrete evidence (`demo-01`/`02`/`03`, `chaos-01`) behind
  the DDD-23 question, resolved in § Amendment above.
