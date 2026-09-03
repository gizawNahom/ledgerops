# ADR-013: Multitenancy migration shape — composite account key, cascading FK correction, single expand-only migration

**Status**: Accepted (DDD-24)
**Owner**: nw-solution-architect (DESIGN wave, `multitenancy`, application leg)

## Context

`feature-delta.md` § Pre-requisites #6 named the physical migration shape as
DESIGN's to settle, and slice-02's own brief recommended a pre-slice SPIKE to
confirm "`(tenant_id, account_id)` composite uniqueness is expressible as an
expand-only migration against `migrations/0001_init.up.sql`'s current bare
`PRIMARY KEY`... without violating the project's expand-only migration
discipline."

Verified directly against `internal/adapters/postgres/migrations/0001_init.up.sql`:
`accounts.id` is a bare, globally-unique `text PRIMARY KEY`. `entries`
references it via two single-column foreign keys, `account_id` and
`counterparty_id`, both `REFERENCES accounts (id)`. `transactions.id` is a
separately, globally-unique `text PRIMARY KEY` (a generated UUID, per
`cmd/api/main.go`'s `IDGenerator`). The project's expand-only discipline
(`brief.md` § Deployment shape) states: "no migration may `DELETE` from or
drop the entry table, and every schema change must leave the previous binary
able to run against the new schema."

US-2's own acceptance criteria require that two different tenants can
independently open an account named `wallet-1` — i.e., account-identifier
uniqueness must become `(tenant_id, id)`-scoped, not global (I9, renumbered
from DDD-18 by `nw-ddd-architect`).

## Decision

**The SPIKE question is answered directly: yes, expressible as a single
expand-only migration — with one structural cascade the SPIKE's own framing
did not name.**

1. **New `tenants` table**, primary key `tenant_id`, `UNIQUE (name)` (I10),
   `UNIQUE (credential_hash)` (credential lookup). `GRANT SELECT, INSERT` only
   — no `UPDATE`/`DELETE`, mirroring OPS-10's least-privilege posture, a free
   consequence of rename/rotation/offboarding being out of scope.

2. **`accounts.id`'s bare `PRIMARY KEY` is replaced by a composite `(tenant_id,
   id)` primary key.** This cannot be avoided or deferred: a global unique
   constraint on `id` alone structurally forbids the exact behavior US-2
   requires (two tenants, same account name). `tenant_id` is added as
   `text NOT NULL DEFAULT 'tnt_legacy_seed' REFERENCES tenants(tenant_id)` —
   the `DEFAULT` (not merely `NULL`-able) is what keeps a hypothetical
   still-running pre-multitenancy binary's `INSERT` (which never mentions
   `tenant_id`) succeeding against the new schema.

3. **Cascading correction, not named by the SPIKE question**: once `accounts.id`
   is no longer independently unique, `entries.account_id REFERENCES
   accounts(id)` and `entries.counterparty_id REFERENCES accounts(id)` are no
   longer valid — Postgres requires an FK target to be a unique/PK column
   set. `entries` gains `tenant_id text NOT NULL REFERENCES
   tenants(tenant_id)`, and both FKs become composite:
   `FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id)`
   and the equivalent for `counterparty_id`. `transactions` gains a plain
   (non-key) `tenant_id text NOT NULL REFERENCES tenants(tenant_id)` — its own
   `id` stays globally unique (UUIDs), no PK change needed there.

4. **One migration file, one transaction, migration-seeded sentinel tenant**
   (`tnt_legacy_seed`) backfills every pre-existing `accounts` row before the
   composite PK constraint is applied. Postgres DDL is transactional; there is
   no hosted environment (dev/CI only, per OPS-1), so a single-transaction
   backfill carries none of the risk the "two-release add-backfill-retire"
   pattern exists to manage for renames under live production load. This is a
   different situation than that pattern governs, not an exception to it.

5. **Expand-only discipline, checked against its literal text, not just its
   spirit**: "no migration may `DELETE` from or drop **the entry table**" is
   not violated by any statement in this migration — no table is dropped, no
   row's content is deleted or rewritten, only columns are added and two FK
   constraints are widened from a stricter (global) to a looser
   (tenant-scoped) shape. The broader "previous binary keeps running" property
   holds up to, but not through, the point a second tenant actually opens a
   duplicate-named account — which cannot happen until this feature's own new
   binary is running and provisioning tenants, so no live pre-multitenancy
   binary is ever exposed to the ambiguity this introduces.

## Consequences

- `internal/adapters/postgres/migrations/0003_multitenancy.up.sql` (next
  sequence number) carries the full shape above in one file.
- `AccountRepository`'s SQL (`LockForUpdate`, `Create`, `ApplyDeltas`, `Get`,
  `All`) must filter and match on `(tenant_id, id)`, not `id` alone —
  mechanical, given the schema above.
- `TransactionRepository`'s `EntriesFor`/`TrialBalance`/`ComputedBalances`
  gain an optional `tenant_id` filter, backed by the new `entries.tenant_id`
  column (no join through `accounts` needed for scoping, which also avoids a
  second query plan for the unscoped case).
- Migrating genuinely existing dogfood data into `tnt_legacy_seed` remains a
  DEVOPS/DELIVER execution concern (per DISCUSS's own scoping) — this ADR
  settles the schema *shape* the backfill runs against, not the backfill
  operation's own execution/rollout plan.

## Rejected alternatives

- **Surrogate UUID row-identity, demoting `id`/`tenant_id` to a plain unique
  constraint instead of the primary key** — rejected. The human-facing
  `{account_id}` path parameter already *is* `accounts.id`; introducing a
  second physical identity purely to dodge a primary-key/FK change duplicates
  identity concepts for no behavioral gain, against simplest-solution-first.
- **Deriving entries' tenant scope via a join to `accounts` instead of a
  stored `entries.tenant_id` column** — considered, rejected: a stored column
  set once (at `Append`, from the same `Post`-produced value that already
  passed the I8 cross-check) cannot independently drift, unlike I3's
  deliberately-independent stored-balance representation; a join-only design
  would cost an extra join on every scoped read for no correctness benefit,
  and this feature has no I3-style reason to want a second, independently
  derived representation to cross-check against.
- **Two-phase migration across two releases (the rename pattern)** —
  rejected as unnecessary for this specific change: that pattern exists to
  protect a live, continuously-serving production deployment through a
  rollout window. No such deployment exists yet (OPS-1: no hosted
  environment), so the risk that pattern manages is not present here.

## References

- `docs/product/architecture/brief.md` § Multitenancy — migration summary,
  cross-referenced from § Application Architecture.
- `docs/feature/multitenancy/feature-delta.md` § Pre-requisites #6.
- `docs/feature/multitenancy/slices/slice-02-operate-within-a-tenant.md` §
  Pre-slice SPIKE — the question this ADR answers directly.
- `internal/adapters/postgres/migrations/0001_init.up.sql`,
  `0002_idempotency.up.sql` — the schema and expand-only precedent this ADR
  extends.
- `adr-011-tenant-partition-not-context.md` — I8/I9/I10's domain-modelling
  origin.
