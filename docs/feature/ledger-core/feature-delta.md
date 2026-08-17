# Feature Delta — ledger-core

Density: `lean` (Tier-1 `[REF]` only). Expansions available on request via
`--expand <id>`.

---

## Wave: DISCUSS / [REF] Persona ID

- **P1 — Integrating developer** (`docs/product/personas/integrating-developer.yaml`)
  Backend engineer building a wallet, marketplace, or points system. Reaches
  ledgerops over HTTP. **Archetype, not researched** — no integrator interviewed.
- **P2 — Platform operator** (`docs/product/personas/platform-operator.yaml`)
  The project owner, dogfooding as sole operator. Real person, real accountability.

Journeys: `docs/product/journeys/post-a-transfer.yaml` (P1),
`docs/product/journeys/verify-the-books.yaml` (P2).

---

## Wave: DISCUSS / [REF] JTBD one-liner

Move value between accounts so that it can never be created or destroyed by
failure, retry, or contention — and prove it.

Full jobs J1–J5 with dimensions, four forces, and opportunity scores:
`docs/product/jobs.yaml`. Scores are founder-estimated, not evidence.

---

## Wave: DISCUSS / [REF] Locked decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Feature type is cross-cutting (fullstack: UI → API → domain → storage) | Slice 01 skeleton must touch every layer |
| D2 | Walking skeleton is Feature 0 — slice 01 | Greenfield; integration risk retired first |
| D3 | UX research depth: comprehensive | UI in scope; emotional arc and error paths earn their keep |
| D4 | JTBD mandatory; every story carries a `job_id` | nWave default; enforces value-outcome framing |
| D5 | Scope split into five slices | Phase 1.5 gate fired 2 oversized signals; user approved split |
| D6 | Synthetic data with documented exception | No production data source pre-launch. Mitigation: property-based tests generate adversarial amounts rather than hand-picked fixtures |
| D7 | **Entries are append-only, enforced at the database level** | Immutability is the precondition for checkpointed verification, period closing, and hash-chain tamper evidence. Cannot be retrofitted onto already-mutated history. Corrections are compensating entries |
| D8 | Single-tenant; tenant isolation deferred | User decision. J6 / I8 removed from scope. Revisit before third-party money |
| D9 | Verification by full scan; checkpointing deferred | Correct and adequate at current volume. `elapsed_ms` instrumentation makes degradation observable |

---

## Wave: DISCUSS / [REF] User stories with elevator pitches

### US-1 — Post a transfer
`job_id: J1` | slice 01 | invariant I1

As an integrating developer, I move value between two accounts and trust it
landed completely or not at all.

**Elevator Pitch**
Before: I have no way to move value between accounts with any guarantee it landed completely.
After: run `curl -X POST /transfers -d '{"from":"a","to":"b","amount":"50.00"}'` → sees `{"transaction_id":"txn_1","status":"posted","legs":[{"account":"a","amount":"-50.00"},{"account":"b","amount":"50.00"}]}`
Decision enabled: whether to tell my user the transfer succeeded.

### US-2 — Reject insufficient funds
`job_id: J3` | slice 02 | invariant I4

As an integrating developer, I never ship a negative wallet balance, even under
concurrent spending.

**Elevator Pitch**
Before: a user can spend more than they hold, and concurrent requests both pass the check.
After: run `curl -X POST /transfers` for more than the balance → sees `422 {"error":"insufficient_funds","available":"10.00","requested":"50.00"}`
Decision enabled: show the user why it failed and how much they actually have.

### US-3 — Retry safely
`job_id: J2` | slice 03 | invariant I7

As an integrating developer, I retry a timed-out transfer without double-charging.

**Elevator Pitch**
Before: retrying after a timeout may post the transfer twice.
After: run `curl -X POST /transfers -H 'Idempotency-Key: k1'` twice → sees the identical `{"transaction_id":"txn_1"}` both times, with one entry pair stored.
Decision enabled: retry on timeout without first checking for a duplicate.

### US-4 — Prove the books balance
`job_id: J4` | slice 04 | invariant I3

As the platform operator, I see at a glance whether the ledger is sound.

**Elevator Pitch**
Before: I have no way to know whether stored balances match recorded entries.
After: open `/console` → sees `Books balance: YES` in words, with per-account reconciliation below; corrupt an entry and it reads `Books balance: NO` naming the account and delta.
Decision enabled: whether to trust the ledger right now, or start investigating.

### US-5 — Trace a balance to its entries
`job_id: J5` | slice 05

As the platform operator, I explain any balance from the entries that produced it.

**Elevator Pitch**
Before: a wrong balance is a number I cannot argue with or explain.
After: run `curl /accounts/{id}/entries` → sees ordered entries with a running balance column, diverging visibly at the offending row.
Decision enabled: identify which entry caused a discrepancy.

**Slice composition gate**: every slice contains at least one user-visible value
story. No slice is `@infrastructure`-only. PASS.

---

## Wave: DISCUSS / [REF] Acceptance criteria

Embedded per story in the slice briefs — `docs/feature/ledger-core/slices/slice-NN-*.md`.
Each brief's AC list verifies its story's Elevator Pitch "After" line end-to-end.

Cross-cutting AC applying to every slice:

- [ ] Every transaction's entries sum to zero, per currency (I1)
- [ ] `UPDATE`/`DELETE` on the entry table are refused by the database (D7)
- [ ] Every slice ships with a `make demo-NN` target that runs on a clean clone

---

## Wave: DISCUSS / [REF] Definition of Done

Authored for this feature — not a framework-canonical list.

1. All slice AC checked and passing
2. Property-based tests present for every invariant the slice claims
3. Adversarial demo runs (`make chaos-`/`race-`/`corrupt-` as applicable)
4. `make demo-NN` succeeds on a clean clone
5. Peer review passed (haiku reviewer per `standard` rigor)
6. Refactor pass completed (L1–L4)
7. No `UPDATE`/`DELETE` path to entries introduced
8. Slice shipped and demoed within one day
9. Learning hypothesis explicitly confirmed or disproved in writing

---

## Wave: DISCUSS / [REF] Out-of-scope

Multi-tenancy and tenant isolation (D8) · multi-currency transactions ·
reversals and refunds · holds, authorizations, reserved balances · overdraft
limits · checkpointed or incremental verification (D9) · idempotency key expiry ·
customer-facing UI · alerting and scheduled checks · repair tooling · export ·
real payment rails · accounting reports, tax, invoicing.

---

## Wave: DISCUSS / [REF] WS strategy

**Strategy C — real local resources** (per Mandate 5).

Justification: the feature's guarantees are properties of the real store's
transactional behaviour. Atomicity, isolation under contention, and append-only
enforcement cannot be validated against an in-memory double — a fake would model
the behaviour we are trying to prove. No costly external dependencies exist, so
Strategy B is unnecessary.

---

## Wave: DISCUSS / [REF] Driving ports

| Port | Surface | Slice |
|---|---|---|
| `POST /accounts` | HTTP | 01 |
| `POST /transfers` | HTTP | 01 (02: 422, 03: `Idempotency-Key`) |
| `GET /accounts/{id}` | HTTP | 01 |
| `GET /accounts/{id}/entries` | HTTP | 05 |
| `GET /health/trial-balance` | HTTP | 04 |
| `GET /console` | Web UI | 04 (05: drill-down) |

All endpoints require the seeded operator API key.

---

## Wave: DISCUSS / [REF] Pre-requisites

- No prior wave artifacts — DISCOVER and DIVERGE were skipped
- `docs/product/` SSOT bootstrapped by this wave
- DESIGN must settle: database and language choice · signed-amount vs
  debit/credit representation · isolation level or locking strategy for I4 ·
  stored balance vs derived-on-read for I3 · idempotency replay storage shape
- DEVOPS receives outcome KPIs only

---

## Wave: DISCUSS / [REF] Outcome KPIs

| KPI | Target | Measurement |
|---|---|---|
| Trial balance integrity | 100% of CI runs report zero on a healthy ledger | CI assertion per build |
| Negative wallet balances | 0 across a 1000-iteration concurrency test | `make race-02` |
| Idempotent posting | Exactly 1 transaction from 50 concurrent same-key submissions | `make race-03` |
| Corruption detection | 100% of injected single-entry corruptions caught | `make corrupt-04` |
| Time to first demo | Clone → `make demo-01` green in under 5 minutes | Timed on a clean machine |
| Slice cycle time | Each slice shipped within 1 day | Recorded per slice |

---

## Wave: DISCUSS / [REF] DoR Validation

Against the canonical 8-item hard gate (`nw-dor-validation`). Note: `nw-discuss`
refers to 9 items; the canonical skill defines 8. Using the canonical list.

| # | Item | Verdict | Evidence |
|---|---|---|---|
| 1 | Problem statement clear and validated | **PASS** | Domain language, real pain, testable — J1–J5 |
| 2 | Persona with specific characteristics | **PARTIAL** | P2 is a real named operator. P1 is an unvalidated archetype |
| 3 | ≥3 domain examples with real data | **FAIL** | Synthetic only, by D6. No production data source exists |
| 4 | UAT scenarios cover happy + edge | **PARTIAL** | Journey error paths documented; Gherkin authored in DISTILL |
| 5 | AC derived from UAT | **PASS** | Slice AC trace to journey steps and error paths |
| 6 | Story right-sized | **PASS** | 5 stories, one per slice, each ≤1 day |
| 7 | Technical notes identify constraints | **PASS** | D7, D9, DESIGN open questions recorded |
| 8 | Dependencies resolved or tracked | **PASS** | Slice dependency chain explicit |

**Result: 5 PASS / 2 PARTIAL / 1 FAIL — DoR does not fully pass.**

Items 2 and 3 fail for the same root cause: no DISCOVER wave ran, so there is no
user evidence and no real data. This is a known consequence of the chosen path,
not an oversight. Handoff to DESIGN requires either a recorded waiver or a
DISCOVER pass to close items 2 and 3.

---

## Wave: DISCUSS / [REF] Wave decisions summary

**Primary jobs**: move value atomically (J1), retry safely (J2), never go
negative (J3), prove the books balance (J4), explain any balance (J5).

**Walking skeleton scope**: slice 01 — one balanced two-leg transfer, HTTP to
real database, with a kill-mid-write chaos demo.

**Feature type**: cross-cutting.

**Constraints established**: entries append-only at database level (D7) ·
single-tenant (D8) · full-scan verification (D9) · synthetic data with PBT
mitigation (D6).

**Upstream changes**: none — no DISCOVER artifacts existed to contradict.

---

## Wave: DESIGN / [REF] DDD list

| ID | Decision | Verdict |
|---|---|---|
| DDD-1 | Modular monolith, ports-and-adapters, pure domain core with I/O at edges | Accepted — testability ranks second |
| DDD-2 | Go 1.23+ for the backend | Accepted — concurrency primitives suit the I4/I7 race tests |
| DDD-3 | PostgreSQL 16 | Accepted — row locks, constraint triggers, real transactions |
| DDD-4 | Console is a separate TypeScript SPA | Accepted — ADR-006; slice-04 ceiling at risk |
| DDD-5 | Entries are signed int64 minor units | Accepted — ADR-001 |
| DDD-6 | `SELECT … FOR UPDATE` in ascending account-id order | Accepted — ADR-002 |
| DDD-7 | Balances stored, updated in the posting transaction | Accepted — ADR-003 |
| DDD-8 | Idempotency: key + fingerprint + transaction_id, replay re-renders | Accepted — ADR-005, supersedes slice-03 recommendation |
| DDD-9 | Append-only entries enforced by the database | Accepted — ADR-004, implements D7 |
| DDD-10 | Paradigm OOP → `@nw-software-crafter` | Accepted — Go routing; idiom is procedural-with-interfaces |
| DDD-11 | One unit of work spans Transaction and Account aggregates | Accepted — deliberate DDD deviation, documented in brief.md |

---

## Wave: DESIGN / [REF] Component decomposition

| Component | Path | Change |
|---|---|---|
| Domain core | `internal/domain/` | CREATE NEW |
| Application / use cases | `internal/app/` | CREATE NEW |
| Ports | `internal/app/ports/` | CREATE NEW |
| Postgres adapter | `internal/adapters/postgres/` | CREATE NEW |
| HTTP adapter | `internal/adapters/http/` | CREATE NEW |
| Entrypoint | `cmd/api/` | CREATE NEW |
| Console SPA | `web/console/` | CREATE NEW |

---

## Wave: DESIGN / [REF] Driving ports

`POST /accounts` · `POST /transfers` · `GET /accounts/{id}` ·
`GET /accounts/{id}/entries` · `GET /health/trial-balance` · Console SPA (browser).
All require an API key.

---

## Wave: DESIGN / [REF] Driven ports + adapters

| Port | Adapter |
|---|---|
| `TransactionRepository` | `postgres` — transaction + entries + balances in one SQL transaction |
| `AccountRepository` | `postgres` — `SELECT … FOR UPDATE`, ascending id order |
| `IdempotencyStore` | `postgres` — unique key constraint, fingerprint + transaction_id |
| `Clock` | `system` / `fake` — injected for deterministic timestamps |
| `IDGenerator` | `uuid` / `fake` — injected for deterministic ids |

---

## Wave: DESIGN / [REF] Technology choices

Go 1.23+ · PostgreSQL 16 · `pgx` v5 · `chi` v5 · `golang-migrate` · `rapid`
(property testing) · TypeScript 5.x. SPA framework deferred to DELIVER — not
architecturally significant.

---

## Wave: DESIGN / [REF] Decisions table

DDD-1 … DDD-11 above. Rationale lives in `docs/product/architecture/adr-00{1..6}.md`.

---

## Wave: DESIGN / [REF] Reuse Analysis

| Existing Component | File | Overlap | Decision | Justification |
|---|---|---|---|---|
| *(none)* | — | — | CREATE NEW | Greenfield. `src/`, `lib/`, `app/` absent; verified by filesystem search. No code exists to extend |

Trivially satisfied for slice 01; non-trivial from slice 02 onward.

---

## Wave: DESIGN / [REF] Open questions

Deferred to DISTILL or DELIVER:

- SPA framework selection (React / Vue / Svelte) — DELIVER
- Idempotency key expiry and garbage collection — known gap, no owner yet
- Currency scale handling once a second currency appears — column exists, logic does not
- Deployment target and CI topology — DEVOPS
- Whether Go↔TypeScript type generation is worth introducing — revisit if the API surface grows

---

## Wave: DESIGN / [REF] Wave decisions summary

**Pattern**: modular monolith with ports-and-adapters.
**Paradigm**: OOP (agent routing); Go idiom is procedural-with-interfaces.
**Key components**: domain core · application · postgres adapter · http adapter · console SPA.
**Stack**: Go 1.23+ · PostgreSQL 16 · TypeScript 5.x.

**Constraints established**: entries append-only at database level (DDD-9) · lock
ordering rule owned by `AccountRepository` (DDD-6) · no external calls inside a
posting transaction · response rendering must be a pure function of the stored
transaction (DDD-8).

**Upstream changes**: slice-03 idempotency recommendation superseded — see
`slices/slice-03-idempotent-retry.md` § Changed Assumptions.

**Outcome collision check**: `nwave-ai outcomes check-delta` → exit 0, no
collisions (registry empty — first feature).
