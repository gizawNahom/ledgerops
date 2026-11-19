---
title: Guarantees
description: How ledgerops keeps balances correct, the mechanism behind each guarantee, and how to check it yourself.
---

Each guarantee below says what it promises, how it is enforced, how to check
it, and where to read the reasoning. The last section says what the checks do
not cover.

The `make` checks need Docker and a running stack (`make up`), and `race-02`
and `race-03` also need Go. The `go test` suites need Docker, because they
start PostgreSQL through Testcontainers. The property tests need only Go.

## How it works

ledgerops is one Go binary in front of PostgreSQL 16. It is a modular monolith
with ports and adapters: a pure domain core decides, and the shell around it
performs the effects.

```text
Integrating service (tenant key)  ─┐
Operator or console (operator key) ─┴─> HTTP API ─> Application ─> Domain core
                                                    (use cases)    (pure, no I/O)
                                                         │
                       ledgerops_app role: no UPDATE or DELETE on entries
                                                         ▼
                                                  PostgreSQL 16
                                                         ▲
                       ledgerops_migrate role: schema changes only
```

- **The domain core is pure.** It takes a request and a snapshot of the
  state it needs, and returns a decision: post, or refuse with a named reason.
  It does no I/O. See
  [ADR-007](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-007-functional-domain-core.md).
- **Two database roles.** The service connects as `ledgerops_app`, which has
  no `UPDATE` or `DELETE` on entries. Schema changes run as a separate
  migration role, and the service is never given that role's connection
  string, so it can't disable the append-only trigger. The design
  ([ADR-018](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-018-single-binary-and-config-contract.md))
  also has `serve` refuse to start if that connection string is in its
  environment. That guard isn't wired up yet.
- **Balances are stored.** An account's balance is a column, updated in the
  same database transaction that writes its entries. It isn't derived on
  read. The books check, served at `GET /health/trial-balance`, compares each
  stored balance with the sum of its entries, which is what lets it notice
  when they disagree. See
  [ADR-003](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-003-stored-balances.md).
- **Migrations only add.** Rolling back the service never needs a schema
  rollback.

### Security model

- **Two kinds of key.** A tenant key reaches that tenant's accounts,
  transfers and entries. The operator key provisions tenants, links tenants
  and runs the books check, and can read entries (`GET /accounts/{id}/entries`)
  and transfers (`GET /transfers/{transfer_id}`) across tenants. It is refused
  on the routes that open accounts, read an account, post transfers and
  register counterparties.
- **A wrong or missing key** gets `401 {"error":"unidentified_caller"}`.
- **`GET /metrics` is unauthenticated** by design, so scrapers don't need a
  key.
- **The console** asks the operator to paste the operator key, and keeps it in
  the browser's local storage. See
  [ADR-010](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-010-console-client-side-api-key.md).
- **There is no TLS in the service itself.** Run it behind a TLS-terminating
  proxy.

## Conservation

**Every transaction's legs sum to zero.** Amounts are signed 64-bit integers
in minor units, so there is no floating-point rounding. Each transfer is built
as two equal and opposite legs, and the domain core checks that the entries sum
to zero before anything is written.

**Check it.** The property-based tests generate random transfers and check
that conservation, refusals that change nothing, and retries that post once
all hold. CI runs them at 1,000 cases each:

```bash
go test ./internal/domain/... ./internal/app/... -race -rapid.checks=1000
```

To check a running ledger, ask for the trial balance. This uses the demo
stack's operator key; see the [quickstart](../quickstart/):

```bash
API=http://localhost:8080
OP="Authorization: Bearer demo-operator-key"
curl -s $API/health/trial-balance -H "$OP"
# {"verdict":"Books balance: YES","imbalance_minor":0,…}
```

**Read more:**
[ADR-001](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-001-signed-int64-minor-units.md),
[ADR-007](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-007-functional-domain-core.md).

## No overdrafts, even under concurrency

**A wallet account's balance never goes negative.** The usual approach is to
read the balance, check it, then write, and under concurrency two requests can
both read a sufficient balance before either writes. ledgerops takes row locks
(`SELECT … FOR UPDATE`) on every account a transfer touches, in ascending
account-id order, before it reads a balance or writes an entry. The fixed
order means two transfers touching the same pair of accounts can't deadlock,
and the second one waits for the first.

This holds for `wallet` accounts. A `system` account, such as the treasury that
funds a wallet, may go negative. That is how value enters the ledger.

The cost is that concurrent transfers touching the same account queue behind
each other. See [Status, limits and operating](../status/#performance).

**Check it.** `make race-02` runs concurrent spenders and prints
`negative_balance_observations`, which should be 0. `make demo-02` prints the
refusal:

```text
422 {"account_id":"alice-01","available":"50.00","error":"insufficient_funds","requested":"500.00"}
```

**Read more:**
[ADR-002](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-002-row-locks-for-sufficient-funds.md).

## Append-only entries

**Entries are never updated or deleted.** `UPDATE` and `DELETE` on entries are
revoked from the service's database role, and a trigger refuses them as a
second layer. A correction is a compensating transaction that reverses the
mistake.

This is enforced by the database for the service's role. It doesn't rely on
every future code path remembering a convention.

**Check it.** The append-only acceptance scenarios run in CI. They need Docker,
because they start PostgreSQL through Testcontainers:

```bash
LEDGEROPS_AT_TAGS=@append-only go test ./tests/acceptance/ledgercore/... -count=1
```

**Read more:**
[ADR-004](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-004-append-only-entries.md).

## Safe retries

**Sending the same request again posts it once.** Every transfer needs an
`Idempotency-Key`. ledgerops stores the key, a fingerprint of the request, and
the resulting transaction id, with a unique constraint on the key written in
the same transaction as the posting. That constraint is what holds under
concurrent duplicates.

| Situation | Status | Response |
|---|---|---|
| New key | `201` | The transfer is posted |
| Same key, same request | `200` | The original transaction, identical body |
| Same key, different request | `409` | `idempotency_key_conflict` |
| No key | `400` | The header is required |

**Check it.**

- `make demo-03` retries a request and reports whether it returns the same
  transaction.
- `make race-03` submits concurrent duplicates and prints
  `distinct_transaction_ids`, which should be 1.
- `make chaos-01` kills the API with `SIGKILL` mid-write, resubmits the same
  key, and prints the balance. A correct run reads 75.00 for `alice-chaos`: one
  25.00 movement, not zero and not two. You compare it yourself.

**Read more:**
[ADR-005](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-005-idempotency-replay.md).

## Tenant isolation

**A tenant's key sees only that tenant's accounts and entries.** A tenant is a
separate set of accounts with its own API key. Account ids are scoped by tenant,
so two tenants can use the same account name without touching each other's
data, and every read and write is scoped to the tenant that the key resolves
to. The operator key can read entries and transfers across tenants.

**Check it.** The multitenancy acceptance suite runs in CI, and needs Docker:

```bash
go test ./tests/acceptance/multitenancy/... -count=1
```

**Read more:**
[ADR-011](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-011-tenant-partition-not-context.md),
[ADR-012](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-012-tenant-credential-mechanism.md),
[ADR-013](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-013-multitenancy-migration-shape.md).

## Cross-tenant transfers need both tenants to agree

**Value moves between two tenants only if both have taken part.** The operator
authorizes a link between the two tenants, with at most one active link per
pair. A tenant then registers the other as a counterparty, which only works
while the link is active, and sends with a `counterparty_alias`. If the link is
revoked, sending to that counterparty is refused.

A cross-tenant transfer can be read only by the two tenants involved, or by the
operator. Any other tenant gets the same `404 transfer_not_found` as for a
transfer that doesn't exist, so a response never reveals that someone else's
transfer exists.

**Check it.** The inter-tenant transfer acceptance suite runs in CI, and needs
Docker:

```bash
go test ./tests/acceptance/intertenanttransfer/... -count=1
```

**Read more:**
[ADR-014](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-014-tenant-link-and-counterparty-alias.md),
[ADR-015](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-015-transfer-coordinator-persistence-and-execution.md),
[ADR-016](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-016-dual-party-transfer-authorization.md).

## What these checks don't cover

:::caution
The service runs the books check on itself, against its own database. It shows
that the entries sum to zero and that each stored balance matches its entries.
It doesn't give an outsider an independent way to confirm that history wasn't
changed.
:::

- **Someone with the migration role or database admin access** can change
  entries. The revoked privileges and the trigger protect against the service's
  own role, not against whoever administers the database.
- **An unreachable database** is reported as unavailable, not as a failed
  check. See
  [ADR-009](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-009-unavailability-is-not-a-refusal.md).
- **Independent verification is on the roadmap:** hash-chained entries, signed
  checkpoints, and a standalone `ledgerops verify` command that an auditor can
  run against a copy of the database without trusting the running service.

For what isn't built yet, see
[Status, limits and operating](../status/).
