# ATDD Infrastructure Policy

Per `nw-distill` § Project Infrastructure Policy. One file per project.
Apply-if-exists; write-if-absent; rewrite with `--policy=fresh`. Git history is
the audit trail.

Created 2026-08-18 during DISTILL of `ledger-core`. Rows are transcribed from
`docs/product/architecture/brief.md` § For Acceptance Designer, which DESIGN
already settled — no port required a soft prompt.

The **Architecture of Reference** decides what KIND of treatment a port class
gets. This file decides the **concrete mechanism** for this codebase. It cannot
override a class default: no driven-internal port may become a fake here.

## Driving

| Port | Mechanism | Note |
|---|---|---|
| `POST /accounts` | Real `chi` router over `httptest.Server`, real API-key middleware | Exercised over the wire, not by calling the handler function |
| `POST /transfers` | Real `chi` router over `httptest.Server` | Carries `Idempotency-Key` from slice 01 onward (DISTILL DDR-1) |
| `GET /accounts/{id}` | Real `chi` router over `httptest.Server` | |
| `GET /accounts/{id}/entries` | Real `chi` router over `httptest.Server` | |
| `GET /health/trial-balance` | Real `chi` router over `httptest.Server` | |
| Console verdict surface | HTTP/JSON only; browser E2E deferred | DISTILL DDR-2. The SPA bundle is not driven by a browser in CI — see § Known gap |
| `cmd/api` binary | `go run ./cmd/api` as a subprocess, for the walking skeleton and the chaos demo | Proves wiring, argument handling, and exit codes — a handler-level test cannot |

## Driven internal (real)

| Port | Mechanism | Note |
|---|---|---|
| `TransactionRepository` (PostgreSQL 16) | Testcontainers `postgres:16`, fresh container per test package (OPS-11) | Never faked — WS strategy C |
| `AccountRepository` (PostgreSQL 16) | Testcontainers `postgres:16`, direct `pgx` pool, **no pooler** | DDD-6 lock ordering needs one session per transaction |
| `IdempotencyStore` (PostgreSQL 16) | Testcontainers `postgres:16`, unique constraint under real concurrent insert | I7 is a property of the constraint, not of the code around it |

Every container is reached through two DSNs (OPS-10): the suite connects as
`ledgerops_app`, and only the corruption and migration helpers connect as
`ledgerops_migrate`.

## Driven external / non-deterministic (fake)

| Port | Fake | Note |
|---|---|---|
| `Clock` | `FakeClock` — a `func() time.Time` literal, manually advanced | Function type per DDD-13, so the fake is one line |
| `IDGenerator` | `FakeIDGenerator` — a `func() string` literal over a fixed sequence | Makes `transaction_id` assertions exact rather than shape-matched |

These two are the only fakes permitted anywhere in the acceptance suite. Both
exist solely so assertions on timestamps and identifiers are deterministic — the
"testability ranks second" quality attribute doing visible work.

**What the fakes cannot model**: `FakeClock` returns whatever the test sets, so
it cannot surface a real clock going backwards, an NTP step, or two postings
genuinely landing in the same nanosecond. Slice 05's same-tick ordering
requirement is therefore tested by *forcing* the collision through `FakeClock`
rather than by hoping for one — which is stronger, but means a real-clock
resolution bug would not be caught here. `FakeIDGenerator` cannot model UUID
collision; that risk is accepted as negligible and untested.

## Known gap — browser E2E

DDD-4 / ADR-006 make the console a separate TypeScript SPA. This policy drives
the console's *contract* over HTTP and does not drive the SPA in a browser
(DISTILL DDR-2). The seam left untested is: SPA fetch wiring, rendering of the
verdict sentence, and the drift-list-to-entries click-through as a user
experiences it. `GET /health/trial-balance` and the console data endpoint are
asserted to return the same verdict, so the *answer* is covered on both paths;
the *presentation* is not. Closing this gap means a ninth CI job and a
Playwright dependency, and is a deliberate deferral, not an oversight.
