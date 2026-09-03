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
| `GET /metrics` (added 2026-08-26, OPS-5 fix) | Real `chi` router over `httptest.Server`, **outside** `requireOperatorKey` | Confirmed unauthenticated 2026-08-26 — mirrors `console_static.go`'s precedent for deliberately unauthenticated read-only surfaces. Scraped presenting no credentials AND presenting a rejected key, both must succeed |
| `POST /tenants` (added 2026-09-03, `multitenancy` DISTILL) | Real `chi` router over `httptest.Server`, mounted behind the existing `requireOperatorKey` group (DDD-22 reuses it verbatim as the platform-admin gate) | RED scaffold today (`scaffold("provision_tenant")`) — the same `httptest.Server` mechanism as every other driving port, no new test-side machinery |
| `POST /accounts`, `POST /transfers`, `GET /accounts/{id}` (extended 2026-09-03, tenant-scoped) | Same `httptest.Server` mechanism, now behind a `tenant_key`-only credential (DDD-22/23) | Existing ports, existing mechanism — only the credential the acceptance suite presents changes |
| `GET /accounts/{id}/entries`, `GET /health/trial-balance`, `GET /console/verdict` (extended 2026-09-03, dual-mode) | Same `httptest.Server` mechanism, exercised twice per scenario set: once with a `tenant_key`, once with the unscoped `OperatorKey` (console-compatibility hard constraint) | No new mechanism — the dual-mode requirement is a scenario-authoring concern (two scenarios, same port), not an infrastructure one |

## Driven internal (real)

| Port | Mechanism | Note |
|---|---|---|
| `TransactionRepository` (PostgreSQL 16) | Testcontainers `postgres:16`, fresh container per test package (OPS-11) | Never faked — WS strategy C |
| `AccountRepository` (PostgreSQL 16) | Testcontainers `postgres:16`, direct `pgx` pool, **no pooler** | DDD-6 lock ordering needs one session per transaction |
| `IdempotencyStore` (PostgreSQL 16) | Testcontainers `postgres:16`, unique constraint under real concurrent insert | I7 is a property of the constraint, not of the code around it |
| `TenantRepository` (PostgreSQL 16, added 2026-09-03, `multitenancy` DISTILL) | Testcontainers `postgres:16`, same fresh-container-per-package mechanism as the three rows above | Not yet built (RED scaffold at the driving port only) — isolation is a property of the real store's `(tenant_id, account_id)` composite-key constraint (DDD-24), so a fake would model the very thing this feature exists to prove (same rationale WS strategy C already states for `AccountRepository`) |

Every container is reached through two DSNs (OPS-10): the suite connects as
`ledgerops_app`, and only the corruption and migration helpers connect as
`ledgerops_migrate`.

## Driven external / non-deterministic (fake)

| Port | Fake | Note |
|---|---|---|
| `Clock` | `FakeClock` — a `func() time.Time` literal, manually advanced | Function type per DDD-13, so the fake is one line |
| `IDGenerator` | `FakeIDGenerator` — a `func() string` literal over a fixed sequence | Makes `transaction_id` assertions exact rather than shape-matched |
| `Logger` (`log/slog`, added 2026-08-26, OPS-5 fix) | `logCapture` — a mutex-safe `io.Writer` behind `slog.NewJSONHandler`, output-captured | Stands in for stdout, where the real handler writes in production (`cmd/api/main.go`). Captured per-request (`logs.linesFrom(marker)`) for field assertions and as a full corpus (`logs.rawText()`) for the release-blocking secret-absence scenarios. `Deps.Logger` is a RED scaffold until DELIVER wires a request-logging middleware onto it (`internal/adapters/http/request_logging.go`) |
| `apiClient`'s `fetch()` (`web/console/`, added DISTILL `ledger-core-console` 2026-08-25) | `vi.fn()` mock returning a scripted `Response`-shaped object | Component-level unit tests only — no real network in jsdom. The real-I/O HTTP/JSON contract this client renders is already asserted over a real Postgres-backed server by `tests/acceptance/ledgercore/milestone-04-proof-of-balance.feature`; this mock proves `apiClient`'s own header/timeout/401 wiring, not the wire contract |
| `keyStorage`'s `window.localStorage` (`web/console/`, added DISTILL `ledger-core-console` 2026-08-25) | None — jsdom's real, spec-compliant `Storage` implementation | Not faked: jsdom's `localStorage` is a real synchronous key-value store, the same API surface the browser exposes. No mock needed for this port |

These two Go-side fakes plus the two TS-side entries above are the only
fakes permitted anywhere in the acceptance suite. All four exist solely so
assertions on non-deterministic or out-of-process state are deterministic —
the "testability ranks second" quality attribute doing visible work.

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
