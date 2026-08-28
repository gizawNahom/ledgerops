# RCA — Observability Status (OPS-5) Falsely Reported as Built

**Investigator**: Rex (nw-troubleshooter) · **Date**: 2026-08-26
**Status**: DRAFT — pending user approval and peer review

## Problem Statement (Scope)

`docs/feature/ledger-core/feature-delta.md` § "Wave: DEVOPS / [REF] Observability
stack" (line 811) and `docs/evolution/2026-08-21-ledger-core.md` § "Handoff to
operations" (line 417-418) both assert `GET /metrics` is "Exposed; nothing
scrapes it yet." Both documents (line 810 / line 902) also assert structured
`log/slog` logging is "Implemented in DELIVER." Neither claim matches the
running code. Scope is bounded to these two status claims in the ledger-core
feature's DEVOPS/Observability record — it does not extend to the console
feature's separate observability posture (out of scope, different feature
delta) or to any other status claim in `feature-delta.md`.

## Evidence Collected (verified directly, not taken on report)

| # | Claim | Verification | Result |
|---|---|---|---|
| 1 | `/metrics` returns real Prometheus exposition | `internal/adapters/http/router.go:65` — `protected.Get("/metrics", scaffold("metrics exposition"))` | FALSE — 501 `__SCAFFOLD__` |
| 2 | Router's own doc comment agrees | `router.go:14-15` — "GET /metrics remains a scaffold: no active scenario exercises it yet" | Ground truth exists in-code, contradicts doc |
| 3 | `prometheus/client_golang` dependency present | `grep -i prometheus go.mod go.sum` → exit 1, no matches | FALSE — dependency absent |
| 4 | Per-request structured fields (`request_id`, `route`, `status`, `elapsed_ms`, `transaction_id`, `account_ids`, `amount_minor`, `currency`, `idempotency_key_hash`, `replayed`, `violation_kind`) are emitted | `grep -rn "slog\." internal/adapters/http/` → zero hits | FALSE — none emitted anywhere in the HTTP adapter |
| 5 | `log/slog` "Implemented in DELIVER" | `cmd/api/main.go` — 4 calls total: `health.startup.refused`, store-open failure, `ledgerops listening`, `server stopped`. All lifecycle-only, zero request-scoped fields | Partially true (slog is wired) but the field set the same table promises does not exist |
| 6 | Request-logging middleware exists | `router.go` — only middleware registered is `requireOperatorKey` (line 57); no second `Use()` call | FALSE — no logging middleware at all |
| 7 | DISTILL authored an acceptance scenario for `/metrics` | `grep -rl metrics tests/acceptance/ledgercore/*.feature` → no matches across all 7 `.feature` files | Confirmed — zero scenarios ever targeted OPS-5 |
| 8 | DELIVER roadmap ever scheduled OPS-5 work | `grep -ni "metric\|prometheus\|slog\|structured.log\|request_id" docs/feature/ledger-core/deliver/roadmap.json` → no matches across all 25 steps | Confirmed — OPS-5 never became a roadmap step |
| 9 | Finalize commit's own verification scope | `1833967` commit message: "Two stale status corrections... exhaustive-linter (DDD-12)... R-2" — named, targeted corrections only | Confirmed — Observability table not named, not swept |
| 10 | Same false claim re-emitted in a second artifact | `docs/evolution/2026-08-21-ledger-core.md:417-418`, authored the same finalize pass (`1833967`, 2026-08-21/22) | Confirmed — error propagated into the evolution doc, not just carried forward unedited in feature-delta.md |
| 11 | Contrast: other unbuilt endpoints ARE correctly flagged in the same finalize output | `2026-08-21-ledger-core.md` "What did NOT ship": explicitly states `/health/trial-balance` and `/console/verdict` "remain `501 __SCAFFOLD__`" | Confirms the finalize process CAN and DOES catch scaffold status when the artifact uses matching vocabulary — see WHY 5A below |

Timeline (git log, verified): `e234e6b` (2026-08-18, DEVOPS decision) →
`516d535` (2026-08-19, DISTILL scaffolds route, no scenario) → DELIVER
commits `c08afb0`+ (2026-08-20+, builds only roadmap-scheduled work) →
`1833967` (2026-08-21/22, finalize, corrects 2 *other* stale claims, does not
audit Observability table).

Your prior investigation (git log + direct reads) is confirmed accurate on all
five numbered points. Extension beyond it: (a) the false claim is duplicated
verbatim in a second artifact (`docs/evolution/...`) authored in the same
finalize pass, not just left stale in `feature-delta.md`; (b) the finalize
pass demonstrably *can* catch unbuilt-endpoint claims — it did so correctly
for `/health/trial-balance` and `/console/verdict` in the same document, using
the shared `__SCAFFOLD__` vocabulary — which pinpoints why Observability
specifically slipped through (WHY 5A); (c) the Logs row ("Implemented in
DELIVER") is a second, distinct false claim riding in the same table, not
merely a supporting detail of the metrics claim.

## Alternative Hypotheses Considered and Ruled Out

Before committing to the finalize-verification-gap root cause, three
alternative explanations were checked directly against the repo (not assumed):

1. **`feature-delta.md` is auto-generated/templated, and the false claim is a
   generator bug, not an authoring gap.**
   Ruled out — `git log --follow` on `docs/feature/ledger-core/feature-delta.md`
   shows eight organic, hand-written wave commits (`826536e` bootstrap →
   `249b575` DESIGN → `1d8b0bb` DISCUSS → `e234e6b` DEVOPS → `516d535`/`f74aff8`
   DISTILL → `36cb6b4`/`e5c451c` DELIVER demo-evidence records), each with a
   distinct, hand-composed commit message describing wave-specific content.
   No template or generator script for this file exists anywhere in the repo
   (`find . -iname "*template*" -o -iname "*generate*feature-delta*"` →
   no matches). This is hand-authored documentation, not generated output.

2. **This is a systemic pattern across the project — other features'
   Observability tables are equally stale.**
   Ruled out as a general pattern (though the *mechanism* that allowed it is
   still systemic — see Root Cause A). `docs/feature/ledger-core-console/
   feature-delta.md:1286-1293` — the sibling feature's own "Wave: DEVOPS /
   [REF] Observability stack" section — correctly states "None — inherited
   unchanged. No Prometheus/Datadog/ELK/OpenTelemetry is [introduced]," and
   line 1096 correctly records "Observability/logging | None ... no new
   stack." The console feature's Observability claims match its code (no
   claim of exposure exists to falsify). This shows the *false-claim*
   instance is specific to ledger-core's DEVOPS record — not a
   project-wide habit of overclaiming — while the *underlying gap*
   (no exhaustive, vocabulary-independent finalize sweep) is still the
   correct systemic root cause: console's table happened to make no false
   claim, not because a sweep caught one, but because console truthfully
   had nothing to overclaim (DDD-level scope was "inherited unchanged").
   The absence of a counter-check does not itself distinguish "no error
   existed" from "an error existed and was missed" in general — it only
   does so here because console's own narrative never asserted an
   affirmative "exposed"/"implemented" outcome for observability at all.

3. **Intentional misrepresentation** (someone knew the code didn't do this
   and wrote "Exposed" anyway).
   Not ruled out with certainty — this investigation found no evidence for it
   and treats it as the less likely explanation, but this is a judgment call,
   not a hard negative. Supporting the ordinary-scope-narrowing reading: the
   same finalize commit (`1833967`) demonstrably *did* correctly self-report
   two other statuses as stale and correct them, and the evolution document
   is unusually candid elsewhere about what did NOT ship (explicit
   `501 __SCAFFOLD__` callouts for `/health/trial-balance` and
   `/console/verdict`, explicit "Wave Completion Enforcement... cannot be
   satisfied by this run and is not reported as satisfied"). A document this
   willing to admit shortfalls elsewhere, in the same section of the same
   commit, is inconsistent with deliberate concealment of one specific
   shortfall. This is corroborating evidence, not proof; flagged here as
   **hypothesis, not finding** — if intent matters for the user's purposes
   (e.g., disciplinary/process-integrity concerns beyond fixing the
   documentation), it needs a direct conversation with the finalize commit's
   author, which is outside this investigation's evidence sources.

## Note on Branch Structure

Branches A and B are presented separately because they are distinct
*symptoms* (two different false claims, in two different table rows, about
two different subsystems). They are **not** presented as independent root
causes — WHY 5B explicitly resolves to the same mechanism as WHY 5A. This is
intentional: the evidence does not support two unrelated root causes, and
forcing an artificially distinct WHY 5B would overstate the findings. Readers
should treat Root Cause A as the single fundamental cause, with Branch B
serving as a second, independently verified data point that the same cause
produced a second symptom — strengthening confidence in Root Cause A rather
than sitting beside it as a peer.

## Five Whys — Branch A: Metrics Exposition

- **WHY 1A** (Symptom): Status table says `/metrics` is "Exposed." [Evidence
  #1, #2, #3 — code returns 501, no dependency exists]
- **WHY 2A** (Context): The claim originates as a DEVOPS-wave *design decision*
  record (OPS-5, commit `e234e6b`), not a verified implementation record — the
  table's own heading is "Wave: DEVOPS / [REF] Observability stack," i.e. it
  documents what DEVOPS decided, and the wording was never revised when
  DISTILL/DELIVER didn't build it. [Evidence: `e234e6b` commit message frames
  OPS-5 among nine platform-readiness *decisions*, before any implementation
  existed]
- **WHY 3A** (System): The decision-to-code gap was never closed because no
  acceptance scenario ever exercised `/metrics` (DISTILL, `516d535`, scaffolds
  the route with zero scenario) and no DELIVER roadmap step ever targeted it
  (25/25 steps, zero mentions). Nothing in the pipeline had a pass/fail signal
  whose failure would force the status column to be corrected. [Evidence #7,
  #8]
- **WHY 4A** (Design): OPS-5 was never explicitly scoped in or out of the
  DELIVER run. The evolution doc records three *explicit* DELIVER-wave scope
  decisions (R-2 closed, ad-hoc DEVOPS-platform-layer build in steps
  05-01..05-03, slices 04/05 + console SPA declared OUT) — observability
  instrumentation is named in none of them. [Evidence:
  `docs/evolution/2026-08-21-ledger-core.md` § "DELIVER-wave scope decisions,"
  lines 156-177 — three numbered items at line 160 (R-2), line 166 (platform
  layer / steps 05-01..05-03), line 172 (scope is walking-skeleton + slices
  01-03 only); none of the three names OPS-5, metrics, or logging] It fell
  outside both the slice-driven roadmap scope and the ad-hoc DEVOPS-catchup
  scope (which covered Docker Compose, Makefile, linter, CI workflows — not
  metrics/logging instrumentation), and unlike slices 04/05 it was never
  flagged as
  deliberately deferred. [Evidence: evolution doc "DELIVER-wave scope
  decisions" section, cross-checked against roadmap.json phases 01-07]
- **WHY 5A** (Root Cause): The finalize pass's status-claim verification was
  **reactive and named-target-only**, not an exhaustive sweep of
  `feature-delta.md`'s status claims. Commit `1833967` explicitly corrected
  two claims it had already identified as suspect (DDD-12 linter, R-2) but
  never audited the Observability table. Crucially, the same finalize pass
  *did* correctly identify and flag `/health/trial-balance` and
  `/console/verdict` as `__SCAFFOLD__` in the same document — proving the
  process is capable of catching this class of error when the artifact
  vocabulary matches the code's own scaffold marker
  (`scaffold(...)` / `__SCAFFOLD__` — see router.go doc comment, "GET /metrics
  remains a scaffold"). The Observability table's wording ("Exposed") never
  uses that shared vocabulary, so the one mechanical signal (grep/scan for
  "scaffold"/"`__SCAFFOLD__`" across the doc, which the finalize author
  evidently applied elsewhere) does not fire on this row.
  **ROOT CAUSE A**: No systematic, vocabulary-independent cross-check exists
  between `feature-delta.md`'s per-wave status tables and actual repository
  state (code, `go.mod`, roadmap, acceptance suite) at finalize time — the
  only verification applied is targeted at claims someone already suspects
  are stale, and the one artifact-side signal that does generalize
  (`__SCAFFOLD__` vocabulary matching) is not used consistently across all
  wave sections.

## Five Whys — Branch B: Structured Request-Field Logging

- **WHY 1B** (Symptom): Table says Logs row is "Implemented in DELIVER," and
  the same section lists a full required-field set — none of which is ever
  emitted. [Evidence #4, #5, #6 — zero `slog.` calls in
  `internal/adapters/http/`, no logging middleware, only 4 lifecycle lines in
  `main.go`]
- **WHY 2B**: "Implemented in DELIVER" is true only for the narrowest possible
  reading — `log/slog` as a package/handler is wired (`main.go:35-36`) — but
  the table cell does not distinguish "logger initialized" from "the
  documented field contract is emitted." The two facts were collapsed into
  one status word. [Evidence: `main.go` slog wiring exists; field list in
  lines 815-820 is a separate paragraph with no per-field status marker of its
  own]
- **WHY 3B**: No acceptance scenario or roadmap step ever asserted the
  presence of `request_id`/`route`/`status`/`elapsed_ms`/etc. in emitted logs
  (same grep sweep as Branch A, #7/#8 — the "structured.log"/"request_id"
  search also returned zero roadmap hits), so, as in Branch A, there is no
  pass/fail signal to force correction of the row.
- **WHY 4B**: The required-field list (lines 815-820) reads as an
  already-decided specification embedded directly under the status table with
  no visual or structural separation — a reader (or the finalize author) sees
  one continuous "Observability stack: built" narrative rather than two
  discrete claims (logger exists vs. field contract fulfilled) that need
  separate verification.
- **WHY 5B** (Root Cause): Same structural root as Branch A —
  **ROOT CAUSE B (shares Root Cause A's mechanism)**: no per-field or
  per-clause verification was run against the DEVOPS wave's Observability
  section; the whole section was authored as a single narrative block and
  audited as a whole (i.e., not at all) rather than as a set of independently
  falsifiable claims.

## Cross-Validation

- Root Cause A and Root Cause B do not contradict — both trace to the same
  systemic gap (finalize verification is reactive/named-target, not
  exhaustive/clause-level) applied to two claims in the same table.
- Backwards check: if Root Cause A/B are true, does it produce the observed
  symptom? Yes — a status table authored at decision time, never gated by a
  scenario or roadmap step, and never swept at finalize, will retain its
  original optimistic wording indefinitely regardless of what DELIVER
  actually built. This mechanism explains both the metrics claim and the
  logging-fields claim without additional causes.
- Completeness check: is there a third branch (e.g., malicious
  misrepresentation, tooling bug)? No evidence found for either — commit
  messages and roadmap show ordinary scope-narrowing under time/token
  pressure, not intent to misstate. No further branch is warranted.
- All observed symptoms (false "Exposed," false "Implemented in DELIVER" for
  fields, absence of prometheus dependency, absence of logging middleware,
  absence of roadmap/AT coverage) are explained by Root Cause A/B together.

## Contributing Factors

1. **Vocabulary inconsistency**: `__SCAFFOLD__`/"scaffold" is the project's
   working marker for "not yet built" (used correctly for
   `/health/trial-balance`, `/console/verdict`, and in `router.go`'s own doc
   comment for `/metrics`), but the Observability table uses "Exposed"/
   "Implemented" instead of that marker — defeating grep-based or pattern-
   based review.
2. **DEVOPS-wave-authored-artifact never re-owned in DELIVER**: unlike slices
   04/05 (explicitly declared OUT, tracked in a numbered scope decision), OPS-5
   was simply never mentioned by any DELIVER-wave scope decision — a silent
   omission is harder to catch than an explicit exclusion.
3. **No AT/roadmap traceability requirement for DEVOPS decisions**: DDD-level
   decisions (DDD-17..21) get ADRs and roadmap steps; OPS-level decisions
   (OPS-1..OPS-10) have no equivalent structural requirement to become a
   roadmap step or acceptance scenario before their status can read anything
   beyond "decided."
4. **Finalize pass had a named checklist, not a full-document audit**: commit
   `1833967`'s scope was two specific claims a person had already flagged;
   nothing forced a walk of every status word in every wave-decision table.

## Proposed Fix

### Immediate Mitigation (documentation correction — restores truthful state now)

Correct the two false claims in place. This is a documentation-only change,
requires no code, and should happen regardless of whether the code fix below
is scheduled:

- `docs/feature/ledger-core/feature-delta.md:811` — change "Exposed; nothing
  scrapes it yet" to "**Scaffold — `501 __SCAFFOLD__`, not implemented**;
  `prometheus/client_golang` not yet a dependency."
- `docs/feature/ledger-core/feature-delta.md:810` — change "Implemented in
  DELIVER" to "Logger wired (`log/slog`, 4 lifecycle events only); the
  per-request field contract below (`request_id`, `route`, `status`, etc.) is
  **not emitted anywhere** — no request-logging middleware exists."
- `docs/evolution/2026-08-21-ledger-core.md:417-418` — same correction,
  consistent wording.

(Per Rex's constraints, this document proposes the exact wording; making the
edit is the user's or an implementing agent's action, not this investigation's.)

### Permanent Fix (code — prevents recurrence by making the claim true)

This investigation proposes the design; implementation belongs to
`nw-functional-software-crafter` per this project's paradigm (CLAUDE.md:
functional, hexagonal, DDD-16 — pure domain core untouched, effect-shell
adapters only). No domain-core files are touched by either change below;
everything lives in the effect shell (`internal/adapters/http/`,
`cmd/api/main.go`).

**(a) Real `GET /metrics` endpoint**

- Add `github.com/prometheus/client_golang` to `go.mod`/`go.sum`.
- Replace `protected.Get("/metrics", scaffold("metrics exposition"))` in
  `internal/adapters/http/router.go:65` with a real handler
  (`promhttp.HandlerFor(registry, promhttp.HandlerOpts{})` or equivalent),
  wired through a `Deps.MetricsRegistry` (or package-level registry
  constructed in `cmd/api/main.go` and passed in) so the adapter stays a thin
  effect-shell wiring point, consistent with how `Deps` already threads
  `Clock`/`IDGenerator` as ports.
- Register the series already named in `feature-delta.md:822-826`:
  `ledgerops_postings_total{result}`, `ledgerops_posting_duration_seconds`,
  `ledgerops_insufficient_funds_rejections_total`,
  `ledgerops_idempotent_replays_total`,
  `ledgerops_trial_balance_imbalance_minor`,
  `ledgerops_trial_balance_scan_duration_seconds`, `ledgerops_drift_accounts`.
  Increment/observe these at the same call sites where the corresponding
  outcome is already decided (e.g., after `ledger.Post` returns, in the
  posting handler) — this is instrumentation of the effect shell around an
  existing decision, not new domain logic, so it stays outside the pure core.
- Consider whether `/metrics` should sit inside or outside the
  `requireOperatorKey` group — currently all JSON endpoints require the key
  except the console static assets; a scrape target typically should NOT
  require an operator bearer token (Prometheus can't easily present one
  without extra config). Recommend moving `/metrics` to an unauthenticated
  route (mirroring `console_static.go`'s precedent for deliberately
  unauthenticated read-only surfaces) or gating it behind network-level
  restriction instead of the operator key — **flag for explicit design
  decision, not silently chosen here.**

**(b) Request-logging middleware**

- New middleware function (e.g., `requestLogger(logger *slog.Logger) func(http.Handler) http.Handler`)
  registered via `protected.Use(requestLogger(...))` in `router.go`, alongside
  (order matters — before or after `requireOperatorKey`, decide based on
  whether unauthenticated-caller refusals should also be logged; recommend
  *before*, so `unidentified_caller` refusals are captured too — this
  requires the group structure to change since `/metrics` and other endpoints
  may sit outside the operator-key group).
- Always-on fields: `request_id` (generate or read `X-Request-Id`),
  `route` (chi's route pattern, not raw path, to avoid cardinality blowup),
  `status` (via a `http.ResponseWriter` wrapper capturing the written status
  code), `elapsed_ms` (wrap start/end `time.Now()` using the existing
  `Deps.Clock` port for determinism in tests).
- Conditional fields, populated only on the relevant endpoints/outcomes:
  - `transaction_id`, `account_ids`, `amount_minor`, `currency` — on
    `POST /transfers` and other posting endpoints, read from the
    already-decided domain result (never re-derive in the adapter).
  - `idempotency_key_hash` (SHA-256 or similar, **never the raw key**) and
    `replayed` — on idempotency-bearing paths, computed from the header value
    already read by the existing idempotency handling; the raw key itself
    must never be passed to `slog`.
  - `violation_kind` — on any response mapped through the sealed
    `domain.ViolationKind`/wire-vocabulary switch (DDD-12/DDD-17), reusing the
    existing exhaustive-linted switch rather than adding a second one.
- **Hard constraint, must be enforced in code review and ideally a test**:
  the `Authorization` header (bearer operator key) and any raw idempotency
  key value must never reach `slog.Any`/`slog.String` calls — only the hashed
  idempotency key. This applies on all paths, including request-echo error
  paths (feature-delta.md:819-820 states this explicitly for the idempotency
  key; the same must hold for the API key, which the doc doesn't explicitly
  restate but the "API keys are never logged" sentence covers directly).
- This is bounded-change/effect-shell work: it wraps existing handlers, reads
  already-computed outcomes, and emits log lines — it does not alter
  `internal/domain/` or introduce new business rules, keeping DDD-16
  (pure core / effect shell) intact.

**Process fix (prevents the documentation-drift root cause, not just this instance)**

- Require every `docs/feature/*/feature-delta.md` wave-decision status table
  cell to use one of a small closed vocabulary
  (`Scaffold`, `Partial`, `Implemented`, `Not started`) rather than free text
  — this restores the grep-ability that let the finalize pass correctly catch
  `/health/trial-balance` and `/console/verdict` but miss the Observability
  row.
- At finalize, require an exhaustive pass over every status table cell
  (not just named-suspect claims) cross-referenced against `grep -r
  __SCAFFOLD__`/`scaffold(` in the adapter layer, `go.mod` diffs, and
  `roadmap.json` step coverage, before the finalize commit is made. This is a
  DEVOPS/platform-architect process recommendation, not something this
  investigation implements.

## Files Affected

Documentation (immediate mitigation):
- `docs/feature/ledger-core/feature-delta.md` (lines 810, 811)
- `docs/evolution/2026-08-21-ledger-core.md` (lines 417-418)

Code (permanent fix — proposed, not implemented by this investigation):
- `go.mod`, `go.sum` — add `prometheus/client_golang`
- `internal/adapters/http/router.go` — replace `/metrics` scaffold; add
  request-logging middleware registration; possibly restructure the
  auth-group boundary for `/metrics`
- `internal/adapters/http/` — new middleware file (e.g.,
  `request_logging.go`) and a metrics-registration file (e.g., `metrics.go`)
  wiring the named Prometheus series to existing handler outcome paths
- `cmd/api/main.go` — construct/inject the Prometheus registry and pass
  through `Deps`; no change to the pure domain core

Not affected: `internal/domain/` (pure core, DDD-16) — no domain change is
required or proposed.

## Risk Assessment of the Proposed Fix

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `/metrics` behind operator-key auth blocks real scraping in a future hosted environment | Medium | Medium — defeats the purpose of exposing it | Explicit design decision needed on auth placement before implementation (flagged above, not resolved here) |
| High-cardinality labels (e.g., raw path instead of route pattern, unbounded account_id sets) blow up Prometheus/log storage | Medium | Medium | Use chi's matched route pattern for `route`; do not label metrics by `account_id` (feature-delta.md D8 single-tenant note already forbids per-tenant labels — extend the same discipline to account-level labels) |
| Raw API key or raw idempotency key accidentally logged via a generic error-echo path | Low-Medium if not tested | High — credential/secret leak | Add an explicit regression test asserting `Authorization` and raw idempotency-key values never appear in captured log output, including on 4xx/5xx paths; treat as a release-blocking AT, not an afterthought |
| Request-logging middleware placement relative to `requireOperatorKey` changes existing auth-refusal behavior or ordering | Low | Medium | Middleware wraps, does not replace, existing handlers; add scenario coverage for unauthenticated-caller logging before changing group structure |
| New middleware/metrics code miscounts or double-counts outcomes (e.g., counts an idempotent replay as a fresh posting) | Medium | Medium — silently wrong metrics are worse than none, since dashboards would be trusted | Instrument at the single point each outcome is already decided (reuse the sealed violation-kind switch and the existing replay/accept/refuse branches) rather than re-deriving outcome classification in the middleware |
| Performance overhead of PromHTTP handler / logging middleware under load-test scenarios (`race-02`, `race-03`, 50-way contention) | Low | Low | These are marginal cost; `elapsed_ms` and status-writer wrapping are O(1) per request; validate no regression against existing race/contention ATs |
| Fix scope creep into `internal/domain/` while wiring outcome data (e.g., adding fields to domain error types) | Low | High (violates DDD-16) | Read `account_ids`/`amount_minor`/`violation_kind` from data the adapter already receives from `ledger.Post`'s return value — do not add adapter-driven fields to domain types |
| This RCA's fix section becomes the de facto acceptance criteria without going through DISTILL | Medium | Medium (process, not code) | Per CLAUDE.md, this is DISTILL/DELIVER territory — the acceptance-designer must author scenarios for `/metrics` and structured logging before a crafter implements; this document is input to that gate, not a substitute for it |

## Recommended Next Step

This RCA is diagnostic and design-only, per this agent's mandate. Recommend:
1. User approves the documentation correction (immediate mitigation) —
   trivial, should happen regardless of code-fix timing.
2. **Unresolved design decision, blocking before DISTILL can proceed**:
   whether `GET /metrics` sits inside or outside the `requireOperatorKey`
   group (see Proposed Fix (a)). This RCA recommends unauthenticated,
   mirroring `console_static.go`'s precedent, but does not decide it — route
   this specific question to `nw-platform-architect` or `nw-solution-architect`
   for an explicit ruling before acceptance scenarios are authored, since the
   auth boundary is itself testable behavior DISTILL needs to know before
   writing the `/metrics` scenario.
3. Dispatch to the owning waves for the permanent fix: `nw-acceptance-designer`
   (DISTILL — author scenarios for `/metrics` and structured-log fields,
   closing the coverage gap identified in Root Cause A/B, using the field
   list and hard constraints in this document as input to scenario design,
   not as binding implementation spec) then `nw-functional-software-crafter`
   (DELIVER — implement against those scenarios, not against this document
   directly). Do not implement directly from this RCA document.
