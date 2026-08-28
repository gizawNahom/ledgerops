# OPS-5 Observability Fix — Design Decisions

**Author**: Morgan (nw-solution-architect) · **Date**: 2026-08-27
**Scope**: Lightweight, bugfix-scoped design consult (`nw-bugfix`: RCA → user
review → DELIVER). This is deliberately NOT a full DESIGN-wave pass: no new
component boundary is created, no new ADR is warranted for a router
restructuring inside an already-decided component
(`internal/adapters/http/`, DDD-16/ADR-007 effect shell).

**Inputs read**: `docs/analysis/2026-08-26-observability-status-false-claim-rca.md`,
`docs/feature/ledger-core/feature-delta.md` § Wave: DISTILL / [REF] OPS-5,
`tests/acceptance/ledgercore/milestone-06-observability.feature` (18 scenario
blocks / 23 executed), `tests/acceptance/ledgercore/ledger_observability.go`,
`internal/adapters/http/router.go`, `internal/adapters/http/request_logging.go`,
`internal/adapters/http/handlers.go`, `internal/adapters/http/status.go`,
`cmd/api/main.go`, `docs/product/architecture/brief.md` § Application
Architecture, `go.mod`.

**Authority**: this document answers the five decisions the RCA and
feature-delta's DISTILL section explicitly left open for
solution-architect/platform-architect ruling. All five were confirmed
directly with the user on 2026-08-27. Nothing here reopens any
already-settled fact (unauthenticated `/metrics`, the field list, the metric
series names, the release-blocking secret-never-logged constraint) — those
are binding inputs, not decisions.

---

## Decision 1 — Prometheus registry/collector composition

**Decision**: `internal/adapters/http/metrics.go`, new file. No new package,
no new component in brief.md's decomposition table — this is instrumentation
of the existing HTTP adapter, not a new boundary.

A `Metrics` struct wraps a private `*prometheus.Registry` and exposes only
narrow, named methods — never a raw `prometheus.Counter`/`Histogram`/`Gauge`
to a caller:

```
type Metrics struct { registry *prometheus.Registry; /* 7 collectors, unexported */ }

func NewMetrics() *Metrics
func (m *Metrics) Handler() http.Handler   // promhttp.HandlerFor(m.registry, ...)

func (m *Metrics) ObservePosting(result PostingOutcome, elapsed time.Duration)
func (m *Metrics) ObserveInsufficientFundsRejection()
func (m *Metrics) ObserveIdempotentReplay()
func (m *Metrics) SetTrialBalanceImbalance(minor int64)
func (m *Metrics) ObserveTrialBalanceScanDuration(elapsed time.Duration)
func (m *Metrics) SetDriftedAccounts(n int)
```

`ObservePosting` increments `ledgerops_postings_total{result=...}` AND
observes `ledgerops_posting_duration_seconds` in one call, at the single site
`postTransferHandler` already decides the outcome (`result.Replayed`,
success, or the error branch) — one call site, one method, no risk of the
two series drifting apart or double-counting.

Wiring: `Deps.Metrics *Metrics` (new field, same pattern as the existing
`Deps.Logger` scaffold). `cmd/api/main.go` constructs `apphttp.NewMetrics()`
once and passes it through `Deps`, exactly like `Clock`/`IDGenerator` today.
`NewRouter` registers `router.Get("/metrics", deps.Metrics.Handler())` (see
Decision 4 for exact mount point).

**Series registered** (verbatim from feature-delta.md/RCA, not renamed):
`ledgerops_postings_total{result}`, `ledgerops_posting_duration_seconds`,
`ledgerops_insufficient_funds_rejections_total`,
`ledgerops_idempotent_replays_total`,
`ledgerops_trial_balance_imbalance_minor`,
`ledgerops_trial_balance_scan_duration_seconds`, `ledgerops_drift_accounts`.

**Rejected alternative**: a sibling `internal/adapters/metrics/` package,
parallel to `internal/adapters/postgres/`. Rejected — nothing outside
`internal/adapters/http/` will ever call it (metrics are recorded exactly
where HTTP handlers already decide outcomes), and brief.md's component table
does not carry a standalone metrics component; adding one would be a new
boundary with no second consumer to justify it (simplest-solution-first).

**New dependency**: `github.com/prometheus/client_golang` — OSS, Apache-2.0.
Add to `go.mod`/`go.sum`. No proprietary alternative was considered; this is
the de facto standard Go client for this exposition format.

**Contract shape** (for the crafter's DISTILL/DELIVER declaration, extending
the reuse-analysis table): **bounded-change**. Declared mutation set = the 7
named series, exactly. Aggregate-bounded universe = the single
`prometheus.Registry` constructed once in `NewMetrics()`. Assertion
mechanism the crafter should rely on: single-registration-site discipline
(only `metrics.go` calls `MustRegister`, enforced by code review / a `grep`
guard since Go has no compile-time closed-registry check available here) —
see the residual-risk note at the end of this document for what this does
*not* catch.

---

## Decision 2 — Request-logging field plumbing

**Decision**: context-carried, write-only accumulator (capability injection,
per the effect-isolation mandate), read exactly once by `requestLogger`.

```
// internal/adapters/http/logfields.go (new file)

type fields struct {
    mu sync.Mutex
    m  map[string]any
}

func (f *fields) Set(key string, value any) { ... } // the ONLY method exposed

// unexported context key; contextWithFields / fieldsFrom are internal helpers
func contextWithFields(ctx context.Context) (context.Context, *fields)
func fieldsFrom(ctx context.Context) *fields // returns a no-op *fields if absent,
                                              // never nil — a handler that logs
                                              // outside requestLogger's scope
                                              // (should never happen) degrades to
                                              // a silent no-op, not a panic
```

`fields` exposes **only** `Set` — no `Get`, no iteration, no way for a
handler to read back what another part of the same request already recorded.
This mirrors the "driving ports that only read must not expose write
methods" discipline in reverse: a capability that only *writes* must not
expose *read*. Only `requestLogger` (which constructed the accumulator) gets
the internal, package-private read path to drain it into one `slog` call.

**Sequencing**:
1. `requestLogger` middleware creates the accumulator, stores it on the
   request context via `contextWithFields`, calls `next.ServeHTTP` with the
   new context.
2. Deep inside the call stack — `postTransferHandler`/`transferAnswer`,
   `writeViolation`, `requireOperatorKey`'s rejection branch — code calls
   `fieldsFrom(r.Context()).Set("transaction_id", id)` (etc.) at the exact
   point each value is already known. No value is re-derived; every field
   is read from data the adapter already has in hand (RCA constraint:
   "reuse the sealed violation-kind switch... do not re-derive").
3. After `next.ServeHTTP` returns, `requestLogger` reads its own accumulator
   (the only reader that exists), merges in `request_id`/`route`/`status`/
   `elapsed_ms`, and makes the single `slog.Info`/`slog.Warn` call. This is
   the one and only place in the adapter that calls `slog` for per-request
   fields (lifecycle logging in `main.go` is unaffected and stays separate).

**Required signature changes** (contained entirely within
`internal/adapters/http/`, confirmed with user):
- `writeDomainError(w http.ResponseWriter, err error)` →
  `writeDomainError(w http.ResponseWriter, r *http.Request, err error)`
- `writeRefusal(w http.ResponseWriter, status int, kind string, extra map[string]any)` →
  add `r *http.Request` — needed so `writeRefusal` can call
  `fieldsFrom(r.Context()).Set("violation_kind", kind)` for every refusal
  path uniformly, including `malformed_request`, `missing_idempotency_key`,
  and `idempotency_key_conflict` (none of which flow through
  `writeViolation`/`status.go`'s exhaustive switch, since they're decided in
  the HTTP adapter per brief.md's refusal-taxonomy table, not the domain
  core).
- `writeViolation(w http.ResponseWriter, v domain.Violation)` →
  add `r *http.Request` — sets `violation_kind` from `v.Kind()` at the same
  site the exhaustive switch already decides the wire status, reusing that
  switch rather than adding a second one (RCA constraint).
- All call sites in `handlers.go` update to pass `r` through (they already
  have it in scope in every handler).

**Rejected alternative**: reshaping every handler to return a structured
result the router serializes, instead of the current
`http.HandlerFunc`-writes-directly-to-`w` shape. Rejected —
disproportionate blast radius (every handler signature changes) for no
additional safety the context-accumulator doesn't already provide, and it
would touch code well beyond OPS-5's scope (violates simplest-solution-first
and the bugfix's stated boundary of touching only the effect shell around
this fix).

**Contract shape**: **bounded-change**. The `fields` accumulator's mutation
set is closed to the same field vocabulary the scenarios and
`request_logging.go`'s doc comment already enumerate (`request_id`, `route`,
`status`, `elapsed_ms`, `transaction_id`, `account_ids`, `amount_minor`,
`currency`, `idempotency_key_hash`, `replayed`, `violation_kind`). Nothing
outside that vocabulary should ever be `Set` — not enforced by the type
system (Go's `map[string]any` doesn't close over a key set), so the crafter
should treat unrecognised keys as a code-review concern, matching how the
7-series metrics universe is enforced (Decision 1).

---

## Decision 3 — Middleware ordering and router restructuring

**Decision**: `requestLogger` wraps the entire router, including static
console assets — `router.Use(requestLogger(deps.Logger))` at the top level,
before any grouping. `requireOperatorKey` stays scoped to the `protected`
group, nested *inside* `requestLogger`'s scope, so:

- Every response — success, domain refusal, `unidentified_caller` 401,
  static asset serve, metrics scrape — passes through `requestLogger` and
  gets exactly one log line.
- `requireOperatorKey`'s own rejection branch calls
  `fieldsFrom(r.Context()).Set("violation_kind", "unidentified_caller")`
  before writing the 401 body, so the release-blocking scenario ("An
  unidentified caller's rejection is logged too") is satisfied without a
  second logging path.
- No scenario distinguishes "static asset requests are logged" from
  "they aren't" — logging everything is the simpler code path (one `Use()`
  call, one wrapping decision to reason about) and was the user's explicit
  choice over scoping to the JSON group only.

**Router shape after this change**:

```
router.Use(requestLogger(deps.Logger))

router.Get("/metrics", deps.Metrics.Handler())        // unauthenticated (Decision 4)

router.Group(func(protected chi.Router) {
    protected.Use(requireOperatorKey(deps.OperatorKey))
    protected.Post("/accounts", ...)
    protected.Get("/accounts/{id}", ...)
    protected.Get("/accounts/{id}/entries", ...)
    protected.Post("/transfers", ...)
    protected.Get("/health/trial-balance", ...)
    protected.Get("/console/verdict", ...)
})

mountConsole(router, consoleDistDir)                  // unauthenticated, unchanged
```

This satisfies the regression outline ("every JSON endpoint except the
metrics exposition still requires the operator key") by construction: the
six protected routes stay inside the one group that carries
`requireOperatorKey`; nothing was added to or removed from that group's
membership except `/metrics` leaving it.

**Route pattern for the `route` field**: chi's matched pattern via
`chi.RouteContext(r.Context()).RoutePattern()`, read inside `requestLogger`
*after* `next.ServeHTTP` returns (chi populates the route context as routing
resolves, which happens during `ServeHTTP`) — never the raw `r.URL.Path`,
per the RCA's cardinality constraint and the scenario "the logged route is
the matched pattern, not the raw path."

---

## Decision 4 — `GET /metrics` mount point

**Decision**: top-level, outside the `protected` group, as shown in Decision
3's router shape. Still passes through the top-level `requestLogger`. No
further ruling needed beyond the restructuring above — this was already
confirmed unauthenticated by the user on 2026-08-26; this document just
specifies the concrete code shape (mirrors `console_static.go`'s existing
precedent for deliberately-unauthenticated surfaces).

---

## Decision 5 — Idempotency key hashing

**Decision**: SHA-256, full 64-character lowercase hex digest, no
truncation.

```
func hashIdempotencyKey(key string) string {
    sum := sha256.Sum256([]byte(key))
    return hex.EncodeToString(sum[:])
}
```

Computed in `postTransferHandler`, immediately adjacent to the existing
`key := r.Header.Get("Idempotency-Key")` line. **Hard rule, carried forward
from the RCA and restated here because it is release-blocking**: the local
variable holding the raw key is used only for (a) the app-layer idempotency
lookup it already feeds, and (b) as the sole input to `hashIdempotencyKey`.
It must never be passed to `fieldsFrom(...).Set(...)` under any key name, on
any path — including the malformed-request/decode-error paths that run
*before* the idempotency lookup, since the raw key is read from the header
before body parsing. The two `@release-blocking` scenarios assert this
against the entire captured log corpus, not just the happy path.

Placement: `hashIdempotencyKey` lives in `internal/adapters/http/handlers.go`
(next to `fingerprintTransfer`, which follows the identical "parsed/derived
value goes in, raw value never crosses the boundary" shape) or
`logfields.go` — crafter's call, no architectural significance either way.

---

## Summary of files touched (all within the already-decided effect shell)

| File | Change |
|---|---|
| `go.mod`, `go.sum` | add `github.com/prometheus/client_golang` |
| `internal/adapters/http/metrics.go` | **new** — `Metrics` struct, 7 collectors, `NewMetrics()`, `Handler()` |
| `internal/adapters/http/logfields.go` | **new** — write-only `fields` accumulator, context plumbing |
| `internal/adapters/http/request_logging.go` | fill in `requestLogger` (currently RED scaffold) — drains `fields`, adds `request_id`/`route`/`status`/`elapsed_ms`, single `slog` call |
| `internal/adapters/http/router.go` | move `/metrics` out of `protected` group; add top-level `router.Use(requestLogger(...))`; wire `deps.Metrics.Handler()` |
| `internal/adapters/http/handlers.go` | `postTransferHandler` calls `Metrics.ObservePosting`/`ObserveInsufficientFundsRejection`/`ObserveIdempotentReplay`, sets `transaction_id`/`account_ids`/`amount_minor`/`currency`/`idempotency_key_hash`/`replayed` on the accumulator; `writeDomainError`/`writeRefusal` gain `r *http.Request` param; add `hashIdempotencyKey` |
| `internal/adapters/http/status.go` | `writeViolation` gains `r *http.Request` param; sets `violation_kind` on the accumulator at the existing exhaustive-switch site |
| `internal/adapters/http/router.go` (`requireOperatorKey`) | sets `violation_kind="unidentified_caller"` on the accumulator before writing 401 |
| `cmd/api/main.go` | construct `apphttp.NewMetrics()`, pass through `Deps.Metrics`; pass `Deps.Logger: logger` explicitly (currently unset — falls back to `slog.Default()`, which happens to work today since `main.go` calls `slog.SetDefault(logger)`, but explicit wiring is clearer and matches how `Clock`/`IDGenerator` are threaded) |
| `internal/domain/` | **untouched** — no domain-core file is modified by any decision above (DDD-16/ADR-007 preserved) |

**Earned Trust check** (explicit, per the mandate): no new external substrate
is introduced. `/metrics` renders an in-process registry over HTTP (no
outbound call, no filesystem, no subprocess); logging writes through the
existing `log/slog` → stdout path `probeStartup`'s neighborhood already
assumes works. Nothing here needs a new `probe()` — the existing
`health.startup.refused` DB probe is unaffected and untouched.

**External integration note for platform-architect handoff**: `/metrics` is
an inbound/driving surface (something scrapes *us*; we call nothing
outbound). No consumer-driven contract test (Pact or otherwise) is
recommended for this fix — there is no new outbound dependency to protect
against a breaking change on the other side.

---

## Residual risk — flagged, not resolved by this design

The metrics scenario "the scrape succeeds... and the exposition names every
declared series" is a **positive** assertion only: it checks the 7 named
series are present, not that *no other* series has been registered. Nothing
in the current 18 scenario blocks would catch a future change that
accidentally registers an 8th series (e.g., a stray default Go-runtime
collector left un-suppressed, or a copy-pasted collector with a typo'd
name that coexists with the correct one instead of replacing it).

This is not something this design document resolves — DISTILL is already
closed and reviewed (4/4 approved), and authoring a new negative-assertion
scenario is DISTILL's call, not solution-architect's. Mitigation available
to the crafter without touching the acceptance suite: keep the
single-registration-site discipline in `metrics.go` tight (one file, one
`NewMetrics()` constructor, no `MustRegister` calls anywhere else in the
package) so the *code* stays auditable by inspection even though the *test
suite* doesn't assert the closed set. Recorded here so it isn't silently
lost between waves.
