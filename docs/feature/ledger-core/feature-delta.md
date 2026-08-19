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
After: run `curl -X POST /transfers -H 'Idempotency-Key: <caller-generated>' -d '{"from":"a","to":"b","amount":"50.00"}'` → sees `{"transaction_id":"txn_1","status":"posted","legs":[{"account":"a","amount":"-50.00"},{"account":"b","amount":"50.00"}]}`
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

## Wave: DISCUSS / [REF] DoR Waiver

**Recorded 2026-08-18, retroactively.** DESIGN and DEVOPS both ran without this
waiver in place. That is a process defect: the DoR section above stated its own
condition for handoff and the condition was not met before handoff occurred.
Recorded here rather than quietly closed, because the sequence is part of the
record.

**Item 2 — persona with specific characteristics (PARTIAL) → waived.**
P2, the platform operator, is a real named person with real accountability, and
P2 owns both operator-facing slices (04, 05). P1, the integrating developer, is
declared an archetype in the persona file itself (`personas/integrating-developer.yaml`)
rather than presented as researched. The risk this leaves open is concrete and
bounded: P1's *ergonomic* preferences — error message shape, response envelope,
key naming — are guesses. P1's *correctness* requirements are not guesses, because
they are double-entry bookkeeping invariants that hold regardless of who is
integrating. The waiver covers the first category only.

**Item 3 — ≥3 domain examples with real data (FAIL) → waived.**
D6 already decided this: no production data source exists pre-launch, and the
compensating control is property-based tests generating adversarial amounts
rather than hand-picked fixtures. That control is stronger than the requirement
it replaces for this specific feature — three real transactions would exercise
three points in the amount space, while the `rapid` suite over the pure `Post`
function (DDD-14) explores it. The waiver is not "we have no data, proceed"; it
is "generated adversarial data covers the invariants better than a small real
sample would."

**What the waiver does not cover.** Neither justification survives contact with
a third-party integrator. D8 already draws that line for tenancy ("revisit before
third-party money"), and the same trigger applies here: before ledgerops accepts
a caller who is not the author, items 2 and 3 need a real DISCOVER pass, not a
second waiver.

**Residual risk accepted by**: the project owner (P2), who is also the sole
consumer at this stage — which is the specific circumstance that makes the
waiver defensible rather than merely convenient.

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
| DDD-10 | ~~Paradigm OOP → `@nw-software-crafter`~~ | **Reversed 2026-08-18** — superseded by DDD-16. See § Changed Assumptions |
| DDD-11 | One unit of work spans Transaction and Account aggregates | Accepted — deliberate DDD deviation, documented in brief.md |
| DDD-12 | Invariant violations are a sealed taxonomy, not open errors | Accepted — Go lacks sum types; closed set via unexported interface, `(value, error)` shape retained |
| DDD-13 | Driven ports hybrid by arity: function types for single-operation, interfaces for transaction-scoped | Accepted — follows effect structure, not paradigm purity |
| DDD-14 | The posting rulebook is one pure `Post` function over locked snapshots | Accepted — I1 and I4 decided in one pure unit; primary PBT target |
| DDD-15 | Strict immutability: unexported fields, smart constructors, no mutating methods | Accepted — invariants hold by construction |
| DDD-16 | Paradigm functional → `@nw-functional-software-crafter` | Accepted — supersedes DDD-10; hexagonal DDD with a functional domain |
| DDD-17 | The sealed refusal set is one wire vocabulary with three declared decision sites; status follows the site | Accepted — ADR-008; extends DDD-12, which sealed one site and read as though it sealed all three |
| DDD-18 | Opening an account that is already open is refused `account_already_exists` / 409, never replayed | Accepted — ADR-008; no caller-supplied key exists, so retry and name collision are indistinguishable and guessing is unsafe |
| DDD-19 | Malformed payloads split at the purity boundary: unreadable → 400 `malformed_request`, illegal → 422; currency mismatch is a domain refusal | Accepted — ADR-008; gives the already-declared 400 a reachable cause |
| DDD-20 | An unreachable or exhausted store is outside the sealed set — 503 `service_unavailable`, no in-request retry, no circuit breaker | Accepted — ADR-009; a refusal asserts nothing moved, and a lost connection cannot assert that |
| DDD-21 | On an unreachable store the trial-balance verdict is **absent**, never `Books balance: NO` | Accepted — ADR-009; `NO` means "I looked", and accusing the ledger of corruption because the database is down destroys the KPI-4 signal |

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

DDD-1 … DDD-16 above. Rationale lives in `docs/product/architecture/adr-00{1..7}.md`.
DDD-12 … DDD-16 are recorded in `adr-007-functional-domain-core.md`.

---

## Wave: DESIGN / [REF] Reuse Analysis

| Existing Component | File | Overlap | Decision | Justification |
|---|---|---|---|---|
| *(none)* | — | — | CREATE NEW | Greenfield. `src/`, `lib/`, `app/` absent; verified by filesystem search. No code exists to extend |

Trivially satisfied for slice 01; non-trivial from slice 02 onward.

### Contract shape per component (Effect Isolation 12 / 6(a))

Declared at design time so the crafter does not have to infer the assertion
mechanism — and so DISTILL's per-scenario `@contract-shape:` tags have something
authoritative to agree with. Recorded 2026-08-18 following DESIGN review.

| Component | Contract shape | Universe | Assertion mechanism |
|---|---|---|---|
| `internal/domain/` — `Post`, `Money`, `Account`, `Entry`, violations | **pure-function** | return value only | Return-only equality plus `rapid` properties over I1/I4. No effect assertion needed: the type signature takes time and identity as values, so a write is non-representable |
| `internal/app/` — `PostTransfer`, `CreateAccount` | **bounded-change** | the touched accounts' balances, the new transaction and its entries, the idempotency record | `assert_state_delta` over port-exposed names; everything in the universe not named in `expected` must be unchanged |
| `internal/app/` — `GetBalance`, `GetEntries`, `VerifyBooks` | **unbounded-preservation** | the entire ledger | These read and must mutate nothing. `VerifyBooks` is the sharp one: a verification that repaired what it found would destroy the cross-check that gives slice 04 its value |
| `internal/adapters/postgres/` — entry writes | **unbounded-preservation** on existing rows, bounded-change on append | all previously recorded entries | D7: `INSERT` only. The store enforces it (OPS-10), so the shape is structural rather than asserted — but CI job 6 asserts the refusal anyway, because a guarantee nobody checks is a guarantee nobody has |
| `internal/adapters/postgres/` — balance writes | **bounded-change** | the stored balances of the touched accounts | Deltas returned by the pure core, applied under the locks the shell took (DDD-6) |
| `internal/adapters/http/` — `POST` routes | **bounded-change** | delegated — the adapter adds no mutation of its own | Encoding and status mapping only |
| `internal/adapters/http/` — `GET` routes | **unbounded-preservation** | delegated | A read route that writes is the bug class this classification exists to make non-representable. The read ports expose no write method, per the split-driving-ports rule |
| `ports.Clock` · `ports.IDGenerator` | **pure-function** from the core's view | n/a | Injected as values; the core reads no clock and generates no id (DDD-14) |

The one place the shapes are load-bearing rather than descriptive:
`VerifyBooks` and every `GET` route are unbounded-preservation, which is what
forbids a future "verify and repair" convenience from being added to the read
path. Repair is out of scope by DISCUSS, but scope decisions erode and type
shapes do not.

---

## Wave: DESIGN / [REF] Refusal taxonomy and status mapping

Authoritative. Rationale: `adr-008-refusal-taxonomy-boundary.md`. Added
2026-08-19 to close AT-completeness gaps C2b and C6a, which were routed here as
`SPECIFICATION_AMBIGUITY`.

DDD-12 sealed the set of ways the ledger says no. It sealed it at one site — the
pure core — and was read as though it sealed all three. The domain's
`ViolationKind` holds four members; the wire vocabulary the caller sees holds
nine, because `unidentified_caller`, `missing_idempotency_key` and
`idempotency_key_conflict` were always decided outside the core. DDD-17 states
the whole set and names each member's site, so a question that lands between
sites has somewhere to be answered.

| Member | Decided at | Status | Body carries | Status |
|---|---|---|---|---|
| `malformed_request` | HTTP adapter | 400 | `field`, `detail` | **NEW** |
| `missing_idempotency_key` | HTTP adapter | 400 | — | existing |
| `unidentified_caller` | HTTP adapter (auth middleware) | 401 | — | existing |
| `account_not_found` | Domain core — `Post` | 404 | `account_id` | existing, **name settled** |
| `account_already_exists` | Domain core — `OpenAccount` | 409 | `account_id` | **NEW** |
| `idempotency_key_conflict` | Application shell | 409 | — | existing |
| `invalid_amount` | Domain core — `NewMoney` / `Post` | 422 | `amount` | existing |
| `insufficient_funds` | Domain core — `Post` | 422 | `available`, `requested` | existing |
| `currency_mismatch` | Domain core — `Post` | 422 | `from_currency`, `to_currency` | **NEW** |

`unbalanced` remains a `domain.ViolationKind` member and is deliberately **not**
a wire member: it guards the Transaction smart constructor against a defect in
the rulebook, and no caller input can reach it. If it escapes, that is a bug in
`Post`, answered 500.

**One existing member disagreed with itself and is settled here, not changed.**
The wire value for the unknown-account refusal is `account_not_found`:
`post-a-transfer.yaml` § error_paths and `internal/domain/violation.go` both say
so, and the acceptance suite's mirror (`domain_types.go:93`) says
`unknown_account`. Two artifacts against one, and the two are the journey the
behaviour was promised in and the scaffold it is decided in. The Go identifier
stays `UnknownAccount` everywhere; only the string on the wire is settled. The
mirror needs a one-line correction — flagged to whoever owns the suite, not
edited by DESIGN.

**Status follows the decision site**, so the next member needs no debate:

- **400** — the request could not be understood as a command
- **401** — the caller could not be identified
- **404** — the command named something that does not exist
- **409** — the identifier supplied is already bound to something else
- **422** — the command was understood and the rules refuse it

**C2b — opening an account that is already open** (DDD-18). Refused
`account_already_exists` / 409, naming the account. Decided in the pure core;
a unique constraint on the account name makes the answer hold under concurrent
opens, exactly as the unique key constraint does for I7. Not idempotent success:
`POST /accounts` carries no caller-supplied key, so the service cannot tell a
retry from a name collision between two independent callers, and guessing
"retry" hands the second caller an account somebody else created.

**C6a — malformed payloads** (DDD-19), split at the purity boundary:

| Sub-case | Answer |
|---|---|
| Invalid JSON · missing required field · unknown field | `malformed_request` · 400 |
| Non-numeric amount (`"abc"`, `null`, wrong JSON type) | `malformed_request` · 400 |
| Over-scale amount (`50.001`) or beyond int64 minor units | `invalid_amount` · 422 |
| Two accounts in different currencies | `currency_mismatch` · 422 |

The adapter parses the *lexical* form of an amount and hands it to `NewMoney`,
which decides legality — scale is a property of the currency, and the currency
is domain knowledge. `currency_mismatch` is a domain refusal because I1 is
per-currency: two legs in different currencies cannot sum to zero per currency
for any amount, so it is unsatisfiable by the invariant rather than disallowed
by policy.

`currency_mismatch` is **declared and currently unreachable through the driving
ports** — every account is opened in the ledger's single configured currency and
`POST /accounts` takes no currency field. That unreachability is how
"multi-currency transactions, out of scope" is enforced rather than asserted.
Its coverage sits at layer 1: § PBT obligations already declares "relax the
same-currency assumption" over `domain.Post`, and this member is what that
obligation now asserts against.

**New constraint**: the account table carries a unique constraint on the account
name, created with the table. Adding it later against history containing
duplicates would fail, and migrations are expand-only.

**New constraint**: the `exhaustive` linter must cover two switch surfaces —
`domain.ViolationKind` in the core, and the wire mapping in the HTTP adapter.
The adapter's switch is the one that keeps the status table honest. See
§ Pre-requisites: this control is **outstanding**, not discharged.

**Obligation for DELIVER — the unreachable member must defend itself in place.**
`currency_mismatch` is the only member with no path to it through a driving
port, which makes it the only member a future maintainer will find, fail to
reach, and reasonably propose deleting. Deleting it silently re-opens C6a and
un-enforces "multi-currency transactions, out of scope". When the constant is
added to `internal/domain/violation.go` — it does not exist today; the file
holds four members — it must carry a comment saying, in substance:

> Unreachable through the driving ports today: every account is opened in the
> ledger's single configured currency, so `Post` never sees two. That is
> deliberate — this member is *how* "multi-currency transactions, out of scope"
> is enforced rather than merely asserted, and I1 being per-currency is why it
> is a domain refusal rather than validation. Reachable at layer 1 now (§ PBT
> obligations, "relax the same-currency assumption"), and at layer 3 the day
> `POST /accounts` accepts a currency. Do not delete for being uncovered.

DESIGN does not write production code; the text and its location are recorded
here so the obligation survives to whoever does.

---

## Wave: DESIGN / [REF] Store unavailability

Authoritative. Rationale: `adr-009-unavailability-is-not-a-refusal.md`. Added
2026-08-19 to close the DESIGN half of AT-completeness gap C7a.

**An unreachable store is not a refusal** (DDD-20). A refusal is a decision —
the ledger read the world, applied its rules, and declined — and every refusal
in this feature carries the implicit assertion that nothing moved, which the
acceptance suite makes explicit as `@contract-shape:unbounded-preservation`. A
connection lost after `COMMIT` was sent and before the acknowledgement arrived
may have mutated everything. The answer is therefore not "refused" but "I do not
know — retry", which ADR-005 already makes safe.

| Condition | Status | Body | Header |
|---|---|---|---|
| Store unreachable · connection refused · statement or lock timeout | 503 | `{"error":"service_unavailable"}` | `Retry-After: 1` |
| Connection pool exhausted | 503 | same | `Retry-After: 1` |
| Write fails on a full or read-only volume | 503 | same | `Retry-After: 1` |

`service_unavailable` is not a member of either sealed set. It is an
availability outcome carried in the same envelope so callers parse one shape,
and there is no switch over it, so DDD-12's `exhaustive` obligation does not
extend to it. Pool exhaustion answers 503 rather than 429: an exhausted pool is
a property of the service's capacity, not the caller's rate, and no rate policy
exists.

**The verdict withholds; it never accuses** (DDD-21). `GET
/health/trial-balance` against an unreachable store answers 503 and never
`Books balance: NO`. `NO` means "I looked, and the books do not balance".
`verify-the-books` already declares the fallback when the console is
unreachable; this declares the answer when the ledger is.

**Encoding — "absent" means the field is omitted, and the whole verdict envelope
with it.** Not `"verdict": null`. The two are different claims: `null` is a
value, and it says "I have a verdict and it is nothing", which is the exact
ambiguity DDD-21 exists to remove. A 503 body is not a trial-balance answer with
a hole in it; it is a different kind of answer.

| Status | Body |
|---|---|
| 200 | `{"verdict": "Books balance: YES" \| "Books balance: NO", "imbalance_minor": …, "entry_count": …, "elapsed_ms": …, "drifted_accounts": […]}` |
| 503 | `{"error": "service_unavailable"}` — and nothing else |

So on 503 there is no `verdict`, no `imbalance_minor`, no `entry_count`, no
`elapsed_ms`. On 200 `verdict` is **always present and never null**; a client
may treat its absence as conclusive proof the ledger was not read. That is the
property worth having, and `null` would destroy it.

The console's third state follows from the status code and the `error` member,
never from inspecting a verdict field: 200 → render YES or NO; 503 → render
*cannot reach the ledger*. A console that branched on the verdict field would
have to decide what a missing verdict means, which is how `NO` gets rendered by
accident — the failure ADR-009 calls the most likely to be implemented without
anyone choosing it.

**No in-request retry, no circuit breaker.** A retry inside the request is a
fresh transaction and a fresh lock acquisition under DDD-6, lengthening the
window the race suite exists to characterise; the caller already holds the
correct retry primitive. A circuit breaker has one dependency and nothing
downstream to protect from cascade. Both rejected on merit, recorded so the
absence reads as a decision.

**Wire, then probe, then use.** `cmd/api/` probes the store before the server
accepts a connection and refuses to start on failure, emitting
`health.startup.refused`. The probe asserts two things: that the app-role
connection can open a transaction and read, and that `UPDATE` on the entry table
as `ledgerops_app` is refused (OPS-10). CI job 6 proves the *migration set* is
right; it proves nothing about the database the binary is pointed at. A
re-granted privilege, a DSN aimed at the migrate role, or a snapshot predating
the role split each yields a service whose append-only guarantee is discipline
in a database costume, invisible until the day it matters.

**Owed to DEVOPS** (raised here, not decided): `environments.yaml` has no
degraded environment. The DESIGN-side preconditions a `degraded` environment
must satisfy are — the store reachable at first `Given` and stopped mid-scenario
rather than absent from the start (so the scenario proves the answer, not the
boot path); a pool sized below the concurrency under test, which is the exact
inverse of `contended`'s precondition and must not be confused with it; and the
observability contract distinguishing a 503 from a refusal, so a spike in
unavailability is never counted as a spike in rejections.

---

## Wave: DESIGN / [REF] Open questions

Deferred to DISTILL or DELIVER:

- SPA framework selection (React / Vue / Svelte) — DELIVER
- Idempotency key expiry and garbage collection — known gap, no owner yet
- Currency scale handling once a second currency appears — column exists, logic does not
- **A `currency` field on `POST /accounts`** — the one change that would make
  `currency_mismatch` (DDD-19) reachable at layer 3 rather than only at layer 1.
  Not taken here: it is an API surface expansion and DISCUSS excluded
  multi-currency. Owner: DISCUSS, when a second currency is wanted
- **503 vs 429, revisit trigger** — DDD-20 answers pool exhaustion with 503 on
  the grounds that capacity is a property of the service and no rate policy
  exists. **The day a rate limit, quota, or throttle is introduced, 429 becomes
  correct for that and 503 stays correct for this**, and the two must be split
  rather than one silently absorbing the other. Recorded because the reasoning
  is conditional on an absence, and absences are the thing nobody re-checks.
  Trigger: the first rate-limiting decision. Owner: DESIGN at that point
- **A transfer whose `from` and `to` name the same account** — noticed while
  closing C6a and deliberately left open. I1 holds trivially and the balance is
  unchanged, so it is neither obviously a refusal nor obviously a legal no-op.
  Adjacent to the audited gaps, not one of them. Owner: DESIGN, next pass
- ~~Deployment target and CI topology — DEVOPS~~ **Closed** by OPS-1..OPS-3
- Whether Go↔TypeScript type generation is worth introducing — revisit if the API surface grows

---

## Wave: DESIGN / [REF] Wave decisions summary

**Pattern**: modular monolith with ports-and-adapters.
**Paradigm**: functional — pure core, effect shell, immutable domain types (DDD-16).
**Key components**: domain core · application · postgres adapter · http adapter · console SPA.
**Stack**: Go 1.23+ · PostgreSQL 16 · TypeScript 5.x.

**Constraints established**: entries append-only at database level (DDD-9) · lock
ordering rule owned by `AccountRepository` (DDD-6) · no external calls inside a
posting transaction · response rendering must be a pure function of the stored
transaction (DDD-8) · the pure core performs no I/O, reads no clock, and
generates no identifiers — every non-determinism is passed in as a value
(DDD-14) · domain types are immutable with unexported fields (DDD-15) · the
account table carries a unique constraint on the account name, created with the
table (DDD-18) · the `exhaustive` linter covers two switch surfaces, the core's
`ViolationKind` and the adapter's wire mapping (DDD-17) · the composition root
probes the store before serving and refuses to start on failure (DDD-20).

**Reopened and closed 2026-08-19** — three `SPECIFICATION_AMBIGUITY` gaps routed
back from DISTILL's AT completeness audit (C2b, C6a, C7a). Settled by DDD-17..21
in § Refusal taxonomy and status mapping and § Store unavailability, recorded in
`adr-008-refusal-taxonomy-boundary.md` and
`adr-009-unavailability-is-not-a-refusal.md`. Three wire members added, one
availability outcome declared outside the sealed set. No previously declared
behaviour was changed.

**Upstream changes**: slice-03 idempotency recommendation superseded — see
`slices/slice-03-idempotent-retry.md` § Changed Assumptions.

**Outcome collision check**: `nwave-ai outcomes check-delta` → exit 0, no
collisions (registry empty — first feature).

---

## Wave: DEVOPS / [REF] OPS decisions

| ID | Decision | Rationale |
|---|---|---|
| OPS-1 | No hosted environment yet — `clean` + `ci` are the entire matrix | All six outcome KPIs are CI assertions, not production telemetry. `brief.md:65` deferred the target here; the honest answer is that nothing needs hosting until money is real |
| OPS-2 | Docker Compose, single Go binary + one PostgreSQL 16 | `brief.md:63` already assumes it for the clean-clone demo path. Kubernetes has nothing to orchestrate |
| OPS-3 | GitHub Actions | A Docker-capable runner is all WS strategy C needs; PostgreSQL 16 comes from Testcontainers (OPS-11), so there is no infrastructure to run |
| OPS-4 | Greenfield — no existing infra or CI to integrate | Verified: repo holds `.git`, `.gitignore`, `CLAUDE.md`, `docs/`, `.nwave/` and nothing else |
| OPS-5 | Structured JSON logs (`log/slog`) + Prometheus exposition at `/metrics`, nothing scraping yet | Instruments cost little now and a lot to retrofit. A deployed metrics stack with no environment to observe would be furniture |
| OPS-6 | Recreate deployment; rollback is redeploy-previous-tag | One binary, one database, one operator, no traffic to protect |
| OPS-7 | No continuous-learning capability (A/B, flags, canary analysis) | Conditional on existing monitoring/alerting infrastructure. There is none — foundational only |
| OPS-8 | Trunk-based development | DoD items 8 and 3 already demand a shipped, demoed slice per day. That is trunk cadence; the branching model should say so rather than contradict it |
| OPS-9 | Nightly-delta mutation testing | Confirmed as-written in `CLAUDE.md`. Delivery is never blocked by mutation runtime, which protects the one-day slice budget |
| OPS-10 | Two database roles: `ledgerops_app` (no `UPDATE`/`DELETE` on entries) and `ledgerops_migrate` (DDL) | D7 says append-only is enforced *at the database level*. A trigger alone is defeated by `ALTER TABLE … DISABLE TRIGGER`; revoked privileges on the role the service actually uses are not. **New constraint — see § Changed Assumptions** |
| OPS-11 | Adapter tests get PostgreSQL 16 from Testcontainers, not a CI service container | One code path for `clean` and `ci` instead of two that drift. Fresh instance per package satisfies the `contended` and `corrupted` isolation preconditions structurally. Detail under § CI/CD pipeline outline |

---

## Wave: DEVOPS / [REF] Environment matrix

Machine artifact: `docs/feature/ledger-core/devops/environments.yaml`. DISTILL
parametrizes acceptance scenarios over these.

| Environment | Platform | Preconditions | Exercises |
|---|---|---|---|
| `clean` | linux · macos · wsl | docker compose available · no prior volumes · schema at migration 0 | All demos; KPI-5 |
| `ci` | GitHub Actions ubuntu-latest | Docker daemon available (Testcontainers, OPS-11) · app role lacks `UPDATE`/`DELETE` on entries · migrations applied by the migrate role first · PBT seed logged | Every push; all KPI gates |
| `populated` | linux · macos · wsl | ≥1 prior transaction and entry pair · trial balance zero on entry · migrations expand-only | Slice 04 scan, slice 05 traceability, migration safety |
| `contended` | linux · macos · wsl | direct `pgx` pool, **no pgbouncer** · single instance, no read replica · pool size ≥ concurrency under test | `make race-02` (I4), `make race-03` (I7) |
| `corrupted` | linux · macos · wsl | reached from `populated` · corruption applied via the migrate role · one account per injection, delta recorded | `make corrupt-04`, KPI-4 |

The nWave defaults (`with-pre-commit`, `with-stale-config`) model install-time
coexistence and are dropped: ledger-core is a service, not an installer. The
five above model where a ledger's guarantees actually break — empty database,
existing immutable history, concurrent writers, and deliberate damage.

Two preconditions are load-bearing and easy to lose. **Pool size ≥ concurrency**
in `contended`: a pool smaller than the concurrency level serialises the test at
the connection layer and turns I4 green without ever contending a row. **No
external pooler**: transaction-scoped repositories (DDD-13) assume the lock
acquired by `SELECT … FOR UPDATE` and the eventual `COMMIT` share one session.

---

## Wave: DEVOPS / [REF] CI/CD pipeline outline

Two workflows. Trunk-based means the full gate suite runs on **every** push to
every branch — not a reduced set on branches and the real thing on `main`. A
gate that only runs post-merge is a gate that reports damage.

`.github/workflows/ci.yml` — trigger: `push` (all branches) + `pull_request`

| # | Job | Needs | PostgreSQL | Gate |
|---|---|---|---|---|
| 1 | `lint` | — | no | `gofmt -l` empty · `go vet` · `golangci-lint` incl. **exhaustive** over violation-kind switches (DDD-12) · `tsc --noEmit` · `eslint` |
| 2 | `build` | — | no | `go build ./...` · SPA production bundle |
| 3 | `unit` | — | no | `go test -race ./internal/domain/... ./internal/app/...` — no database reachable from this job, by design |
| 4 | `property` | — | no | `rapid` suite, ≥1000 checks; seed printed to log and uploaded as an artifact on failure (DDD-14 determinism is worthless if the seed is lost) |
| 5 | `integration` | 1,2 | Testcontainers | Migrations from zero, then adapter tests |
| 6 | `append-only-proof` | 5 | Testcontainers | `UPDATE` and `DELETE` on entries as `ledgerops_app` are **refused** — asserts both the revoked privilege and the trigger (cross-cutting AC, D7) |
| 7 | `invariant-gates` | 5 | Testcontainers | `make race-02` · `make race-03` · `make corrupt-04` · trial-balance zero. Emits KPI-1..4 to the job summary |
| 8 | `demo` | 2 | docker compose | `make demo-01 .. demo-NN` from a clean checkout; wall-clock emitted as `demo_first_green_seconds` (KPI-5) |

Jobs 1–4 need no database and run in parallel from the start; 5–8 fan out after
the build. Branch protection on `main`: all eight required, linear history, no
force-push.

**OPS-11 — adapter tests obtain PostgreSQL 16 via Testcontainers**, not a
GitHub Actions service container. The runner's Docker daemon is the only CI
dependency. Rationale: a service container is declared in workflow YAML and has
no local equivalent, so the `clean` and `ci` environments would be wired two
different ways and drift silently — the failure mode being a test that passes
locally against a developer's long-lived database and fails in CI, or worse, the
reverse. Testcontainers makes both environments the same code path. It also
satisfies the `contended` precondition structurally: a fresh instance per test
package cannot have a pooler in front of it, and cannot inherit dirty state from
a previous test, which matters most for `corrupt-04` where leftover drift would
produce a false pass. Cost, accepted: Docker becomes mandatory for any adapter
test, and cold start adds a few seconds per package. Job 8 keeps `docker compose`
because the demos must exercise the same path a developer's clean clone does.

`.github/workflows/nightly.yml` — trigger: `schedule` (daily) + `workflow_dispatch`

| Job | Scope | Gate |
|---|---|---|
| `mutation-delta` | Go files changed in the last 24h (`go-mutesting` or equivalent) | Kill rate reported, **not** blocking (OPS-9) |
| `slice-cycle-time` | git metadata → `slice-NN-shipped` tags | Reported (KPI-6) |
| `demo-cold` | Docker layer cache disabled | Cold-start `demo_first_green_seconds`, the honest KPI-5 reading |

No `deploy` job exists. There is nowhere to deploy (OPS-1), and a stub deploy
job that no-ops is worse than its absence — it reads as coverage that is not
there.

---

## Wave: DEVOPS / [REF] Monitoring contracts

Full contract with field names and thresholds: `docs/product/kpi-contracts.yaml`.

| KPI | Target | Instrument | Assertion | Blocks build |
|---|---|---|---|---|
| Trial balance integrity | 100% of runs zero | `GET /health/trial-balance` → `imbalance_minor`, `entry_count`, `elapsed_ms` | `imbalance_minor == 0` | yes |
| Negative wallet balances | 0 over 1000 iterations | `make race-02` → `negative_balance_observations`, `iterations` | `negatives == 0 AND iterations >= 1000` | yes |
| Idempotent posting | exactly 1 txn from 50 | `make race-03` → `distinct_transaction_ids`, `entry_pairs_stored` | both `== 1`, `submissions >= 50` | yes |
| Corruption detection | 100% caught | `make corrupt-04` → `detected`, `account_named_correctly`, `delta_correct` | all `== injections` | yes |
| Time to first demo | < 5 min | `demo` job wall-clock → `demo_first_green_seconds` | `< 300` | yes |
| Slice cycle time | ≤ 1 day | git first-commit → `slice-NN-shipped` tag | none — reported | no |

Three of these assert a *denominator* alongside the result. `iterations >= 1000`
and `submissions >= 50` exist because a harness that quietly ran ten iterations
reports zero negatives and passes. Likewise KPI-4 asserts attribution, not just
detection: US-4 promises the verdict names the account and the delta, so a bare
`NO` would satisfy a detection-only check and still fail the operator.

KPI-6 is deliberately ungated. Gating cycle time rewards under-scoping a slice,
not working faster.

Guardrail: `elapsed_ms` on the trial-balance scan warns above 2000ms. That is
the agreed trigger to revisit D9's deferred checkpointing — a signal, not a
failure.

---

## Wave: DEVOPS / [REF] Deployment strategy

**Recreate.** Stop the container, start the new one. One binary, one database,
a single operator, and no traffic whose interruption anyone would notice.
Blue-green and canary both presuppose a traffic router and a second stack; with
one PostgreSQL holding append-only history, the database is the hard half of a
blue-green swap and duplicating it is precisely what D7 forbids.

**Rollback contract.** Rollback is redeploy of the previous image tag, and it is
constrained by immutability. Migrations are **expand-only**: a migration may add
tables, columns, and indexes, and may never `DELETE` from or drop the entry
table. Consequently every schema change must leave the *previous* binary able to
run against the *new* schema — new columns arrive nullable or defaulted, renames
are add-then-backfill-then-stop-writing across two releases, never `ALTER …
RENAME`. Rolling back code is therefore always safe and never requires rolling
back schema, which is the only rollback story compatible with a ledger that
cannot forget. Contracting migrations, when eventually needed, are a separate
deliberate release after the old binary is retired.

---

## Wave: DEVOPS / [REF] Mutation testing strategy

**nightly-delta** (OPS-9), unchanged from `CLAUDE.md` § Mutation Testing
Strategy. CI runs mutation on files modified that day; it does not run during
feature delivery and does not block a slice.

Reconciliation, since the two config surfaces look contradictory:
`.nwave/des-config.json` sets `rigor.mutation_enabled: false`, which governs
whether the **DELIVER wave** runs mutation inline. Under nightly-delta it
correctly stays false. The nightly CI job is the mutation surface. No file
changes; the apparent conflict is the two settings describing different things.

Primary mutation target is `internal/domain/` — the pure `Post` function
(DDD-14) is where I1 and I4 are decided, and it is the one place where a
surviving mutant means an invariant is untested rather than merely under-covered.

---

## Wave: DEVOPS / [REF] Observability stack

| Signal | Choice | Status |
|---|---|---|
| Logs | `log/slog`, JSON to stdout | Implemented in DELIVER |
| Metrics | `prometheus/client_golang`, exposition at `GET /metrics` | Exposed; **nothing scrapes it yet** (OPS-1) |
| Traces | none | Declared out |
| Alerting | none | Out of scope per DISCUSS (`feature-delta.md:140`) |

Required log fields: `ts`, `level`, `msg`, `request_id`, `route`, `status`,
`elapsed_ms`; plus `transaction_id`, `account_ids`, `amount_minor`, `currency`
on postings, `idempotency_key_hash` and `replayed` on idempotency paths, and
`violation_kind` on rejections. The idempotency key is logged **hashed** — it is
client-chosen and may carry caller-side identifiers. API keys are never logged,
including on request-echo error paths.

Series: `ledgerops_postings_total{result}`, `ledgerops_posting_duration_seconds`,
`ledgerops_insufficient_funds_rejections_total`,
`ledgerops_idempotent_replays_total`,
`ledgerops_trial_balance_imbalance_minor`,
`ledgerops_trial_balance_scan_duration_seconds`, `ledgerops_drift_accounts`.

Tracing is omitted on merit, not budget: one process, one database, no network
hop between components. The single meaningful span is already reported by
`posting_duration_seconds`. Revisit when a second service exists.

---

## Wave: DEVOPS / [REF] Branching strategy

**Trunk-based development.** `main` is the only long-lived branch; work branches
live under a day and merge behind the full eight-job gate. Tag `slice-NN-shipped`
on the commit that completes each slice — KPI-6 is computed from those tags, so
the convention is a DELIVER obligation, not decoration.

Alignment with CI: because branches are short-lived and merge continuously, the
identical suite runs on every push (see pipeline outline). Branch protection on
`main` requires all eight jobs plus linear history. This is the CI cost
trunk-based development trades for: the gate must be trustworthy on every commit,
because there is no release branch in which to discover problems later.

---

## Wave: DEVOPS / [REF] Coexistence matrix

| Tool | Must not break | Note |
|---|---|---|
| `make demo-01` … `demo-05` | yes | Cross-cutting AC. Slice 05 breaking slice 01's demo is a CI failure in the same build, not a follow-up ticket |
| `docker compose` dev stack | yes | The clean-clone path depends on it (`brief.md:63`) |
| `golang-migrate` | yes | Migrations stay plain reviewable SQL; no ORM-generated schema alongside |
| `pre-commit` | yes | Not installed today. Listed so that adding hooks later does not silently displace an existing `core.hooksPath` |

The first row is the one that bites. Every slice's demo target stays in the CI
`demo` job permanently, so the demo suite grows monotonically and regression is
caught by the build that causes it.

---

## Wave: DEVOPS / [REF] Pre-requisites

DESIGN constraints the platform must satisfy:

- **WS strategy C** — every environment touching an adapter runs real
  PostgreSQL 16. No in-memory variant is offered anywhere in CI
- **DDD-6 lock ordering** — direct `pgx` pool only; no pgbouncer or external
  pooler may sit between the service and PostgreSQL in any environment where
  `race-02`/`race-03` run
- **D7 append-only** — enforced by *both* revoked role privileges (OPS-10) and a
  rule/trigger, with job 6 asserting the composite refusal
- **DDD-12 / DDD-17 exhaustiveness** — `golangci-lint` must run the `exhaustive`
  linter over **two** switch surfaces: `domain.ViolationKind` in the core, and
  the wire-mapping switch in `internal/adapters/http/` that turns a violation
  into a status and an error name. Covering one does not cover the other — a
  member added to the core with no mapping at the wire is exactly the defect the
  sealed set exists to prevent, and only the second surface catches it.
  **Corrected 2026-08-19 by DESIGN: this was written as "now discharged" and is
  not.** CI job 1 is a design; no `.github/workflows/` and no `.golangci.*`
  exist in the repository. The second surface did not exist when the original
  claim was written (DDD-17 created it), but the first was never wired either.
  Still owned by DEVOPS, still outstanding
- **DDD-14 determinism** — PBT seeds printed to the CI log every run and
  uploaded as an artifact on failure. A non-reproducible property failure in a
  ledger is a bug you cannot chase
- **D9 full-scan verification** — `elapsed_ms` surfaced on the health endpoint
  and warned on at 2000ms, so degradation is measured rather than guessed
- **Single-tenant (D8)** — no per-tenant labels on any metric or log field.
  Adding them later is a schema change; inventing them now is speculative

---

## Wave: DEVOPS / [REF] Wave decisions summary

**Deployment**: none hosted — Docker Compose locally, Recreate strategy recorded
for when an environment exists, rollback by previous image tag under an
expand-only migration rule.
**CI/CD**: GitHub Actions, eight required jobs on every push, trunk-based.
**Observability**: `slog` JSON logs + Prometheus exposition, no collector, no
traces, no alerting.
**Mutation testing**: nightly-delta, non-blocking, targeting `internal/domain/`.

**Constraints established**: two database roles with `UPDATE`/`DELETE` on
entries revoked from the application role (OPS-10) · no external connection
pooler · migrations expand-only, never deleting entry rows · pool size ≥
concurrency in contended tests · PBT seeds archived · every slice's demo target
retained in CI permanently.

**Open questions closed**: "Deployment target and CI topology — DEVOPS"
(`feature-delta.md:328`) and `brief.md:65` are both resolved by OPS-1..OPS-3.

**Upstream changes**: one — OPS-10 adds a database-role constraint DESIGN did
not specify. Recorded below and propagated to `brief.md`. No architecture
contradiction was found; no `upstream-changes.md` was required.

---

## Changed Assumptions

### OPS-10 — append-only enforcement needs a role split, not only a trigger (2026-08-18)

**Original assumption** — `docs/product/architecture/brief.md:186`, Invariants
table:

> | D7 | Entries are never updated or deleted | Database: revoked privileges plus a rule/trigger |

and `feature-delta.md:36`:

> | D7 | **Entries are append-only, enforced at the database level** | … |

**New assumption**: "revoked privileges" is made concrete as a **two-role
split**. `ledgerops_app` — the role the running service and every test connect
as — holds `SELECT` and `INSERT` on the entry table and has `UPDATE` and
`DELETE` revoked. `ledgerops_migrate` holds DDL and is used only by
`golang-migrate` and by the corruption harness. The service never connects as
the migrate role.

**Rationale**: DESIGN named revoked privileges and a trigger in the same breath
without saying which role loses what, which is answerable only once there is a
deployment and a test harness — that is this wave. The split matters for two
concrete reasons. First, a trigger alone is not database-level enforcement in
any meaningful sense: whatever role can `ALTER TABLE … DISABLE TRIGGER` can
mutate history, and if that is the application's own role the guarantee is
discipline wearing a database costume. Second, `make corrupt-04` must *create* a
genuine drift to prove slice 04 detects it. Under a single role, either the
harness cannot corrupt anything, or the application can — and exactly one of
those is acceptable. The split resolves it: corruption is possible only for the
privileged role the service never uses.

**Downstream impact**: the postgres adapter takes two DSNs rather than one;
`cmd/api/` wiring reads only the app DSN; migrations and the corruption harness
read the migrate DSN. CI job 6 asserts the refusal as `ledgerops_app`.
`brief.md` § Invariants and § Deployment shape updated. DISTILL and DELIVER have
not run, so nothing already built is invalidated.

### DDD-10 reversed — paradigm OOP → functional (2026-08-18)

**Original assumption** — `docs/feature/ledger-core/feature-delta.md`, DESIGN
DDD list, as written on 2026-08-18:

> | DDD-10 | Paradigm OOP → `@nw-software-crafter` | Accepted — Go routing; idiom is procedural-with-interfaces |

and in `docs/product/architecture/brief.md:7`:

> **Paradigm**: OOP (agent routing) — Go idiom is procedural-with-interfaces

**New assumption** (DDD-16): the paradigm is **functional**. Implementation
routes to `@nw-functional-software-crafter`. The domain is modelled as immutable
value types with the posting rulebook expressed as one pure function; the
application layer is an explicit effect shell.

**Rationale**: DDD-10 was decided by language default — `/nw-design` step 4
classifies Go as OOP-native and recommends OOP — rather than by the design's own
shape. The shape already contradicted the label: DDD-1 specified a pure domain
core with I/O at the edges, and `Clock` and `IDGenerator` were already ports so
that the core would be deterministic. That is a functional architecture wearing
an OOP routing token.

Two things made the reversal worth the churn:

1. **The knowledge is agent-bound.** The `nw-fp-hexagonal-architecture`,
   `nw-fp-domain-modeling`, and `nw-fp-principles` skills are declared on
   `@nw-functional-software-crafter` and are not loadable by
   `@nw-software-crafter`. Under DDD-10 the crafter would have implemented a
   pure-core design without access to the pure-core playbook.
2. **Testability ranks second.** A pure `Post` over locked snapshots (DDD-14) is
   directly property-testable with no database and no mocks, which is what the
   I1/I4 invariant suite needs.

**Known cost, accepted**: no `nw-fp-go` skill exists — the installed FP language
skills are F#, Haskell, Scala, Clojure, and Kotlin. The crafter's language
detection will find `*.go`, fail to load a Go-specific FP skill, and fall back to
generic FP guidance. Go's missing sum types are the sharp edge, which is why
DDD-12 settles violation representation explicitly at design time rather than
leaving it to the crafter. Go remains the language: DDD-2 was decided on
concurrency primitives for the I4/I7 race tests, and that reasoning is untouched.

**Downstream impact**: none consumed yet. DEVOPS, DISTILL, and DELIVER have not
run; no production code exists. `CLAUDE.md` § Development Paradigm updated to
match, which is the file `/nw-deliver` step 1.5 actually reads.

---

## Wave: DISTILL / [REF] Inherited commitments

| Origin | Commitment | DDR | Impact |
|--------|------------|-----|--------|
| DISCUSS#D2 | Walking skeleton is slice 01, authored by DISTILL since SPIKE was skipped | n/a | One `@walking_skeleton @driving_port @driving_adapter @real-io` scenario written; it is the only scenario not tagged `@pending`, so DELIVER makes it green first and nothing else is worth reading until it is |
| DISCUSS#D6 | Synthetic data, mitigated by property-based generation of adversarial amounts | n/a | Acceptance scenarios stay example-based (Mandate 9 — they run at layer 3); the generated-adversarial obligation is declared as a PBT table DELIVER owns, so the mitigation is tracked rather than assumed |
| DISCUSS#D7 | Entries append-only, enforced at the database level | n/a | Four `@append-only` scenarios assert refusal as the application role, including an attempt to disable the protection itself — a trigger alone would pass a weaker test |
| DISCUSS#WS-C | Strategy C, real local resources, no in-memory doubles | n/a | Every driven repository runs against real PostgreSQL 16; Tier B in-memory state-machine PBT is deliberately NOT emitted (see § Two-tier decision) |
| DESIGN#DDD-8 | Idempotency replays re-render from the stored transaction | DDR-3 | A scenario compares the replayed legs against the entries actually recorded, which a cached response body would fail; replay status fixed at 200 |
| DESIGN#DDD-13 | Ports hybrid by arity — function types for `Clock`/`IDGenerator` | n/a | Those two are the only fakes in the suite, and the infrastructure policy records what they cannot model |
| DESIGN#DDD-14 | The posting rulebook is one pure `Post` function | n/a | Declared as the primary PBT target for DELIVER; the acceptance layer asserts its consequences at the port, never calls it directly |
| DESIGN#DDD-4 | Console is a separate TypeScript SPA (ADR-006) | DDR-2 | Console scenarios drive the verdict contract over HTTP, not a browser; the SPA rendering is a recorded untested seam |
| DEVOPS#OPS-10 | Two database roles, `UPDATE`/`DELETE` revoked from the app role | n/a | The suite holds two DSNs; corruption is reachable only through the privileged one, which is what makes the slice-04 verdict honest |
| DEVOPS#OPS-11 | PostgreSQL 16 per test package via Testcontainers | n/a | One code path for `clean` and `ci`; a fresh instance per scenario, so `corrupted` cannot pass on leftover drift |
| ADR-005 | `Idempotency-Key` is required, not optional | DDR-1 | Every `POST /transfers` scenario carries a key from slice 01 onward, so slice 03 tightens behaviour without rewriting a single earlier scenario |

---

## Wave: DISTILL / [REF] Scenario list with tags

SSOT for scenarios is the `.feature` files. 7 files · **60 scenarios** (59
blocks; the one `Scenario Outline` expands to 2 examples) · 445 step
invocations. Counts reconciled 2026-08-19 after § AT completeness audit added
six scenarios; the `Scenarios` column below counts executed scenarios, as it
always has. Every scenario except the walking skeleton carries `@pending`
(ADR-025 one-at-a-time; DELIVER unskips, it does not re-author), and every
scenario carries a `@contract-shape:` tag (see § Contract shape classification).

| File | Scenarios | Tags |
|---|---|---|
| `walking-skeleton.feature` | 1 | `@walking_skeleton @driving_port @driving_adapter @real-io @slice-01 @us-1 @env-clean` |
| `milestone-01-post-a-transfer.feature` | 12 | `@us-1 @slice-01` · 6 `@error` · 1 `@chaos @driving_adapter` |
| `milestone-02-sufficient-funds.feature` | 8 | `@us-2 @slice-02` · 4 `@error` · 2 `@env-contended @kpi-2` |
| `milestone-03-idempotent-retry.feature` | 9 | `@us-3 @slice-03` · 4 `@error` · 1 `@chaos` · 1 `@env-contended @kpi-3` |
| `milestone-04-proof-of-balance.feature` | 11 | `@us-4 @slice-04` · 5 `@error` · 5 `@env-corrupted` · 4 `@kpi-4` |
| `milestone-05-entry-traceability.feature` | 10 | `@us-5 @slice-05` · 3 `@error` · 2 `@env-corrupted` |
| `integration-checkpoints.feature` | 9 | `@real-io @adapter-integration` · 4 `@append-only` · 1 `@env-contended` |

**Error coverage: 24 of 60 = 40%** (target ≥40%), under the same counting rule
as before — `@error` plus the two `@env-corrupted` scenarios that exercise
deliberate damage without carrying the tag. Two corrections come with the
recount: the earlier 22 undercounted `@error` in milestone-03 by one (the file
has always held four, the table said three, so the true figure was 23 of 54 =
43%), and the rule was stated as "assert a NO verdict", which never described
one of the two scenarios it counted.

The ratio fell from 43% to 40% because five of the six scenarios added by the
audit are boundary, cardinality and interruption edges — smallest accepted
amount, a key surviving an interruption, an empty ledger's verdict, an empty
trace, a single-row trace — and not one of them is a refusal, so none carries
`@error`. **`@error` is a refusal taxonomy, not an edge taxonomy**, and
reading it as the latter is what makes the drop look like a regression. Counted
as error *and* edge, which is what the ≥40% target is actually about, the figure
is 29 of 60 = 48% and rose.

Story coverage: US-1 → 13 scenarios, US-2 → 8, US-3 → 9, US-4 → 11, US-5 → 10.
Every story has at least one scenario asserting its Elevator Pitch "After" line
end to end.

---

## Wave: DISTILL / [REF] WS strategy

**Strategy C — real local resources**, inherited from DISCUSS unchanged, now
expressed through the Architecture of Reference rather than renegotiated: port
CLASS implies port TREATMENT, and the project policy at
`docs/architecture/atdd-infrastructure-policy.md` records the MECHANISM per port.

One walking-skeleton scenario. Stakeholder litmus test, stated in the feature
file so it survives: *"an integrator creates two accounts, funds one, moves
fifty from it to the other, and both balances say so."* Real HTTP, real domain,
real PostgreSQL, real balances read back.

---

## Wave: DISTILL / [REF] Two-tier decision (Mandate 10)

**Tier A only. Tier B is deliberately NOT emitted**, and this is a judgement
call worth recording rather than a default.

Both Tier B trigger conditions are met on their face — the post-a-transfer
journey chains five scenarios, and the input space (amounts, account pairs,
repeat counts) is domain-rich. Tier B is skipped anyway because it requires an
`InMemoryComposition` standing in for the repositories, and the guarantees this
feature exists to prove — atomicity, isolation under contention, append-only
enforcement — are properties of the real store's transactional behaviour. An
in-memory double would model the very behaviour under test. That is the exact
reasoning DISCUSS used to select strategy C, and it does not stop applying
because the tier changed.

The property surface is not lost, it is relocated: DDD-14 puts it at the pure
`Post` function, where it needs no doubles at all. See the PBT obligations table
below.

---

## Wave: DISTILL / [REF] Adapter coverage table (Mandate 6)

| Adapter | `@real-io` scenario | Covered by |
|---|---|---|
| `TransactionRepository` (postgres) | YES | WS · `integration-checkpoints` append-only ×4 · migration-over-history |
| `AccountRepository` (postgres) | YES | WS · `milestone-02` contended ×2 · `integration-checkpoints` opposing-direction deadlock |
| `IdempotencyStore` (postgres) | YES | `milestone-03` ×9, including 50-way concurrent same-key and one key surviving an interruption |
| `Clock` (fake) | N/A — fake by policy | `integration-checkpoints` "timestamps come from the injected clock" asserts the injection itself, so a clock read from the store would fail |
| `IDGenerator` (fake) | N/A — fake by policy | `integration-checkpoints` "identifiers come from the injected generator", same reasoning |
| HTTP driving adapter | YES | Every scenario; WS via real socket, real routes, real API-key middleware |

Zero `NO — MISSING` rows. The two fakes are the only ports not exercised for
real, they are permitted by the Architecture of Reference (driven external /
non-deterministic), and each has a scenario proving the injection seam is real
rather than decorative.

---

## Wave: DISTILL / [REF] Driving adapter coverage

DESIGN entry points scanned; every one has at least one scenario reaching it
over its own protocol, not by calling a service function.

| Entry point | Exercised by | Verifies |
|---|---|---|
| `POST /accounts` | WS · `milestone-01` | status (201/400/401/409), body, argument handling |
| `POST /transfers` | WS · milestones 01–03 · both contended runs | status (201/200/400/401/404/409/422), body, headers |
| `GET /accounts/{id}` | WS · every balance assertion | status, body |
| `GET /accounts/{id}/entries` | `milestone-05` ×10 | status, body, ordering, empty listing |
| `GET /health/trial-balance` | `milestone-04` ×11 | status, body, `entry_count`, `elapsed_ms` |
| Console verdict surface | `milestone-04` "both surfaces agree" | contract parity with the health surface (DDR-2 — not driven in a browser) |
| `cmd/api` binary | `milestone-01` chaos scenario (subprocess, killed mid-write) | wiring, restart against the same store |

The API-key middleware is implemented for real rather than scaffolded, so the
three unauthenticated-caller scenarios may pass on first run. That is intended:
authentication gates everything else and is not waiting on DELIVER.

**Status lists reconciled 2026-08-19 by DESIGN** (count-only edit to this table;
no verdict, rule or obligation in this section was touched). The declared 400 on
`POST /transfers` previously had no reachable cause, because the only declared
400 was a missing idempotency key. DDD-19 gives it one — `malformed_request`.
Two further corrections: 401 was exercised by the unauthenticated-caller
scenarios without ever being written down, and `POST /accounts` was listed as
though it had no refusal path, which DDD-18 changes. **503 is deliberately
absent from both rows**: DDD-20 declares it, no scenario asserts it, and a
`degraded` environment does not yet exist. Adding it to this table before a
scenario reaches it would claim coverage that is not there.

---

## Wave: DISTILL / [REF] Scaffolds

Mandate 7 — 9 files, all carrying `SCAFFOLD: true`. `grep -rl "SCAFFOLD" internal cmd`
must return zero once DELIVER completes.

| File | Shape |
|---|---|
| `internal/domain/money.go` | `Money`, smart constructor, `Add`/`Negate`/`IsPositive` — panics |
| `internal/domain/account.go` | `Account`, `AccountKind`, `Apply` — panics |
| `internal/domain/violation.go` | Sealed taxonomy (DDD-12), unexported interface — panics |
| `internal/domain/post.go` | `Post`, `Transaction`, `Entry`, `BalanceDelta`, `EntriesSumToZero` — panics |
| `internal/app/ports/ports.go` | Port declarations only — nothing to panic; `UnitOfWork` keeps the three repositories on one transaction |
| `internal/app/usecases.go` | Five use cases — panics |
| `internal/adapters/http/router.go` | **Real routes, real auth middleware; handlers answer `501 __SCAFFOLD__`** |
| `internal/adapters/postgres/store.go` | `Migrate`/`Open` no-op; everything an assertion depends on panics |
| `cmd/api/main.go` | Real wiring, app DSN only |

The two deviations from "everything panics" are deliberate and are what make the
RED classification honest — a panicking HTTP handler drops the connection and a
panicking `Migrate` fails the `Given`, and both classify BROKEN rather than RED.
Full reasoning: `distill/red-classification.md`.

---

## Wave: DISTILL / [REF] PBT obligations for DELIVER

Acceptance scenarios are example-based because they run at layer 3 (real
adapters), per Mandate 9, and sad paths stay enumerated per Mandate 11. The
property surface DISCUSS#D6 and the slice ACs demand lives in DELIVER's unit
tests over the pure core, with `rapid`. Declared here so it is tracked, not
assumed.

| Property | Over | Slice AC it discharges |
|---|---|---|
| For any generated transfer, the returned entries sum to zero per currency | `domain.Post` | slice-01 "PBT generates adversarial amounts and asserts I1" |
| No generated sequence of transfers drives a wallet account negative | `domain.Post` + `Account.Apply` | slice-02 "no generated sequence drives a wallet negative" |
| For any request and any repeat count ≥1, final state equals the state after exactly one application | `PostTransfer` over fakes, plus the real store for I7 | slice-03 "any repeat count" |
| Adversarial amounts — zero, one minor unit, max int64, exact balance, off-by-one-cent | `domain.Money`, `domain.Post` | slice-01 and slice-02 boundary coverage |
| Negative testing (Hebert ch.6): relax the same-currency assumption and the non-empty-snapshot assumption | `domain.Post` | none — surfaces under-specification before a second currency exists |

**Seeds must be printed every run and archived on failure** (DEVOPS
pre-requisites). A non-reproducible property failure in a ledger is a bug you
cannot chase.

---

## Wave: DISTILL / [REF] Test placement

`tests/acceptance/ledgercore/` — Go convention places a suite that needs real
infrastructure outside `internal/`, and the package cannot live under `internal/`
anyway without becoming importable only by the module itself. No precedent
existed to follow: greenfield, verified by filesystem search.

```
tests/common/statedelta/state_delta.go   # polyglot bootstrap, project-wide, once
tests/acceptance/ledgercore/
  *.feature                              # scenario SSOT — 7 files
  domain_types.go                        # Mandate-12 typed nouns
  ledger_world.go                        # composition root: lifecycle, universe, transport
  ledger_observations.go                 # containers, reads, contended runners
  ledger_seeding.go                      # Given-side helpers, all through the driving ports
  ledger_assertions.go                   # Then-side vocabulary
  steps_ledger_test.go                   # godog bindings — one delegating statement each
  suite_test.go                          # runner; default tag filter ~@pending
```

**Runner choice — godog.** DESIGN listed `rapid` for property testing but named
no Gherkin runner, and the polyglot matrix's Go row presumes none. godog was
chosen so the `.feature` files stay executable rather than decaying into prose
that drifts from the tests beside them, and it runs under plain `go test`, so
the eight-job CI pipeline is unchanged. Cost, accepted: one test-only dependency
DESIGN did not name. A test runner is not architecturally significant, which is
the same standard by which DESIGN deferred the SPA framework.

**State-delta port** bootstrapped at `tests/common/statedelta/` (apply-if-absent,
first DISTILL in this project). All eight predicates implemented with the
canonical fail-closed semantics.

---

## Wave: DISTILL / [REF] Mandate-12 compliance

Mechanical criteria, not a ratio:

| # | Criterion | Verdict | Evidence |
|---|---|---|---|
| 1 | Domain types module exists | PASS | `domain_types.go` — 8 typed enums (`AccountKind`, `RefusalKind`, `Outcome`, `Verdict`, `Surface`, `Credentials`, `TamperAction`, plus `Money`/`AccountName`/`IdempotencyKey` newtypes) and 7 observable structs |
| 2 | Composition methods consume typed parameters | PASS | No composition-root method takes a raw `string` where a domain type exists; coercion happens in argument position via `Parse*` |
| 3 | No business logic in step bodies | PASS | 93 step bodies, every one a single delegating statement; `grep -E '^\s+(if\|for\|while\|switch\|select) '` over `steps_ledger_test.go` returns nothing |
| 4 | Step-reuse ratio reported | **3.42×** (318 Gherkin step lines / 93 decorators) | Informational per the refined mandate — this is the natural ceiling for this feature shape, not a miss |

---

## Wave: DISTILL / [REF] Contract shape classification

Designer principle 14 (2026-05-15, identity-essential). Every scenario carries a
`@contract-shape:` tag; the tag is machine-parseable and drives the crafter's
choice of universe mechanism in DELIVER. Added 2026-08-18 following review —
the tags were missing from the first pass and Sentinel blocked on it.

| Shape | Scenarios | What the tag commits the crafter to |
|---|---|---|
| `bounded-change` | 24 | A declared, aggregate-bounded mutation set. `assert_state_delta` over the port-exposed universe; anything declared but not expected must be unchanged |
| `unbounded-preservation` | 35 | The action must mutate **nothing**. Every refusal, every read, every verdict |
| `pure-function` | 0 | Correctly zero at this layer — the pure surface is `domain.Post`, and it belongs to DELIVER's `rapid` suite, not to a scenario that crosses a socket |

59 scenario blocks tagged (godog counts 60 by expanding the one
`Scenario Outline` into its two examples). Mechanical check — the two figures
must be equal, which is the invariant, not the number:

```bash
grep -hE '^\s*Scenario( Outline)?:' tests/acceptance/ledgercore/*.feature | wc -l   # 59
grep -ohE '@contract-shape:[a-z-]+' tests/acceptance/ledgercore/*.feature | wc -l    # 59
```

The classification is by the shape of each scenario's **When**, not its Given.
The corruption scenarios are the case that makes this worth stating: their
`Given` mutates an entry out of band, but the `When` only asks for the verdict,
and asking must change nothing — so they are `unbounded-preservation`. Tagging
them `bounded-change` because damage appears somewhere in the scenario would
license the verifier to write, which is precisely what must never happen.

Authoritative source for the shapes: DESIGN § Contract shape per component,
added in the same review cycle. The two agree by construction.

The typing is what keeps the decorator count at 90 rather than ~130: one
decorator covers the whole sealed refusal taxonomy (`(?:as|for) (.+)` →
`ParseRefusalKind`), one covers both account kinds, one covers every
credentials × tamper-action pair. Collapsing further would mean merging steps
that read differently to a stakeholder, which Pillar 1 outranks.

---

## Wave: DISTILL / [REF] AT completeness audit

Designer Phase 2.5 / DoD item 21 — the `nw-at-completeness-check` 7-category
taxonomy (C1–C7) and its 15-item mechanical checklist, computed over the real
`.feature` set rather than inferred from titles. Run 2026-08-19. It had never
been run: the mandate lives in the agent definition and this wave was executed
skill-driven, the same miss class as R-1.

**Verdict: COMPLETE — 13 of 15 passing** (thresholds: <10 INCOMPLETE · 10–12
acceptable with listed gaps · ≥13 complete). 8 of 15 at first computation; six
scenarios and one docstring closed C1b, C2a and C3, reaching 11. C2b and C6a
closed on 2026-08-19 once DESIGN answered them (DDD-18, DDD-19 / ADR-008) —
see § Closing C2b and C6a below. Two documented gaps remain and are listed as
such: C4a is owed by DELIVER, C7a is blocked on an environment DEVOPS has not
built. A COMPLETE count is not a claim that nothing is outstanding.

| Item | Verdict | Evidence / gap |
|---|---|---|
| C1a — empty / zero / minimum input | PASS | `A new account starts empty` · `The schema builds from nothing` · `the ledger holds no entries` · amount 0.00 refused |
| C1b — each partition boundary | PASS *(closed)* | Sufficiency boundary had exact (10.00 accepted) and over (10.01 refused); the smallest **accepted** amount was absent. Added `The smallest amount the ledger can move is accepted` (0.01). int64 ceiling stays at layer 1 — it is already in § PBT obligations, which is where Mandate 9 puts it |
| C2a — state machine documented | PASS *(closed)* | Store / account / key state machines written out in `ledger_world.go`. Writing them is what surfaced C2b |
| C2b — illegal event from each state | PASS *(closed 2026-08-19)* | Every state has an illegal-event scenario except `account: open`, whose illegal event is *open again* — unspecified upstream, so it was routed rather than invented. DDD-18 answered it (`account_already_exists` / 409, naming the account, deliberately not replayed as idempotent success). Added `Opening an account somebody already opened is refused and their account is untouched` |
| C3 — cardinality 0 / 1 / many | PASS *(closed)* | Trace had many (5) and 2, but neither 0 nor 1; the drift list had 0 and 1 but not many; no scenario asked the verdict of an empty ledger. Added `An account nothing has happened to traces to an empty history`, `An account with a single movement traces to a single row`, `A ledger holding nothing balances, and says so`, `Two damaged accounts are both named` |
| C4a — apply-twice per mutating op | **GAP** *(narrowed)* | Posting is covered exhaustively (5 replay/conflict scenarios); opening an account twice is covered since C2b closed. What remains is one operation only. Re-running the migration set — required idempotent by `environments.yaml § deployment_assumptions` — has no scenario, and one written today would pass vacuously against the empty `MigrationStatements()`, so it is raised for DELIVER rather than authored now |
| C4b — inverse op without prerequisite | PASS *(N/A by design, documented)* | The SUT has no destructive inverse: entries are append-only (D7) and reversals are out of scope. The analogue — acting on an absent prerequisite — is covered three times (transfer in, transfer out, trace, all against an account nobody opened), and the four `@append-only` scenarios assert that attempting the inverse is refused. No reversal scenario was invented for a capability DISCUSS excluded |
| C5a — mode-flag combinations | PASS | No `dry_run`/`force`/`verbose` exists. The decision-table axes that do — account kind × direction × sufficiency, key present/absent/reused-same/reused-different, surface console/health, credentials app/privileged, operator key absent/wrong/valid — are each exercised across their materially-distinct combinations |
| C5b — flag orthogonality | PASS | `The console and the health check give the operator the same answer` (surface changes presentation, never the answer) · `The same request written differently is still the same request` (representation changes nothing about identity) |
| C6a — malformed value per input param | PASS *(closed 2026-08-19)* | Amounts were covered for zero and negative, account names for unknown, the key for absent; nothing submitted a malformed body, a non-numeric or over-scale amount, or a currency mismatch, and the taxonomy did not say what any of those answer. DDD-19 split them at the purity boundary. Added two outlines: seven unreadable-request shapes (`malformed_request` / 400) and two unholdable amounts (`invalid_amount` / 422). `currency_mismatch` is deliberately unscenarioed — see below |
| C6b — each declared error triggered | PASS | All five journey `error_paths` plus both auth refusals have a scenario that triggers exactly that refusal: insufficient funds · unknown account · key conflict · invalid amount · missing key · unidentified caller |
| C6c — closed error set asserted | PASS | The contended scenarios account for every outcome with no residue — `exactly 1 accepted` + `exactly 19 refused for insufficient funds` over 20 attempts, and `every attempt is answered` + `no attempt is answered with a deadlock`. A third outcome would fail them |
| C7a — degraded resource | **GAP** *(still blocked)* | No scenario runs with the store unreachable, the pool exhausted, or the disk full. DESIGN has since declared the answer (DDD-20 / ADR-009: unavailability is an outcome, not a refusal, so `service_unavailable` is **not** a `RefusalKind` member — admitting it would turn `@error` from a refusal taxonomy into an outcome taxonomy). The scenarios remain unwritable: they need a `degraded` environment — store reachable at the first `Given` and stopped mid-scenario, pool sized below the concurrency under test — and DEVOPS has not built one. Owed by DEVOPS, not by DISTILL |
| C7b — interruption mid-operation | PASS | `A transfer interrupted halfway leaves no half-applied movement` (kill mid-write, restart, assert both legs or neither). Expected to be thin and was not — but it left slice-03's reworded AC uncovered, closed below |
| C7c — concurrent actors | PASS | Four contended scenarios: 20 racers on one balance · 1000 contended spends · 50 same-key submissions · 100 opposing-direction movements for lock ordering |

**Six scenarios added, all `@pending`, all `@contract-shape:`-tagged against
DESIGN § Contract shape per component, zero undefined steps.** The set was then
**59 scenario blocks / 60 executed** across the same 7 files; 318 Gherkin step
lines over 93 decorators (step-reuse **3.42×**, informational). Three new
bindings (`no entries are returned`, `exactly N entries are returned`, and a
keyed variant of the kill step), one new assertion method, and one scaffold
signature change (`InterruptPostingMidWrite` now carries the key). `go build
./...` and `go vet ./...` pass.

| Added scenario | File | Shape | Closes |
|---|---|---|---|
| The smallest amount the ledger can move is accepted | milestone-01 | bounded-change | C1b |
| A key and the movement it guards survive an interruption together or not at all | milestone-03 | bounded-change | slice-03 AC (see below) |
| A ledger holding nothing balances, and says so | milestone-04 | unbounded-preservation | C3 (0 entries) |
| Two damaged accounts are both named, not just the first one found | milestone-04 | unbounded-preservation | C3 (many drifted) |
| An account nothing has happened to traces to an empty history | milestone-05 | unbounded-preservation | C3 (0 rows) |
| An account with a single movement traces to a single row | milestone-05 | unbounded-preservation | C3 (1 row) |

The idempotency one is the find worth naming. Slice 03's AC was **reworded by
this wave's own U-1 finding** — from "killing the process between them is not
possible because they are one write" to "after any interruption, a key exists if
and only if its transaction exists, observable by retrying with the same key" —
specifically so a scenario could be written against it, and then no scenario
was. The chaos coverage sat in milestone-01, where the interrupted transfer
carries no key and therefore cannot observe the property. Now it does.

**Gap classification and routing** (per the skill's §5 rule: C2/C5/C6/C7 gaps
whose upstream artefact is absent are `SPECIFICATION_AMBIGUITY` and re-enter the
owning wave, not DISTILL).

| Gap | Kind | Owner | Severity | What is owed |
|---|---|---|---|---|
| C2b — opening an account that is already open | SPECIFICATION_AMBIGUITY | DESIGN (+DISCUSS) | HIGH | The sealed violation taxonomy (DDD-12) has no member for it and the journey's `error_paths` do not list it. Idempotent create? 409? Silent success? Whichever, `POST /accounts` needs it before slice 01 is done. **Answered 2026-08-19** — DDD-18 / ADR-008: refused `account_already_exists` / 409, naming the account, explicitly not idempotent success |
| C6a — malformed body, non-numeric or over-scale amount, currency mismatch | SPECIFICATION_AMBIGUITY | DESIGN | HIGH | § Driving adapter coverage already claims `POST /transfers` answers 400, but no declared refusal produces one for a malformed payload. Currency mismatch is the sharper half: slice 01 says "same currency" and nothing says what happens when it is not. **Answered 2026-08-19** — DDD-19 / ADR-008: split at the purity boundary, `malformed_request` / 400 at the adapter and `invalid_amount` / `currency_mismatch` / 422 in the core |
| C7a — store unreachable / pool exhausted / disk full | SPECIFICATION_AMBIGUITY | DEVOPS (+DESIGN) | MEDIUM | `environments.yaml` has no degraded environment, and no declared answer exists for a ledger whose store is gone. `verify-the-books` already assumes a fallback path when the console is unreachable and never states the API's own failure answer. **Half-answered 2026-08-19** — DDD-20 / ADR-009 declares the answer; the `degraded` environment that would let a scenario reach it is still owed by DEVOPS |
| C4a — migration set applied twice | AT_GAP_IN_DELIVERY_SCOPE | DELIVER | LOW | `deployment_assumptions` requires idempotent, expand-only migrations. Deliberately not authored here: with `MigrationStatements()` returning an empty map it would pass vacuously, and this wave already carries one vacuous pass it has told DELIVER not to count |

### Closing C2b and C6a (2026-08-19)

DESIGN answered both gaps (DDD-18, DDD-19, recorded in
`adr-008-refusal-taxonomy-boundary.md`; reviewed CONDITIONALLY_APPROVED, all
conditions applied). Three scenario blocks were authored against those answers.
All three are `@pending`, all three carry a `@contract-shape:` tag classified by
the shape of their **When**, and zero undefined steps remain.

| Added scenario | File | Tags | Shape | Closes |
|---|---|---|---|---|
| Opening an account somebody already opened is refused and their account is untouched | milestone-01 | `@pending @error` | `unbounded-preservation` | C2b · C4a (open twice) |
| An amount the ledger cannot hold exactly is refused and nothing moves | milestone-01 | `@pending @error` | `unbounded-preservation` | C6a (over-scale · beyond int64) |
| A request the ledger cannot read as a command is refused and nothing moves | milestone-01 | `@pending @error @driving_adapter` | `unbounded-preservation` | C6a (unreadable body · missing field · unknown field · non-numeric, empty and bare-number amounts) |

All three are `unbounded-preservation` and each asserts it: the balances stand
and the entry count is unchanged. That is the point of the C2b one in
particular — a refused second open must leave the *first* caller's account
exactly as it was, which is the failure DDD-18 refuses to risk by guessing
"retry".

**`currency_mismatch` gets no scenario, deliberately.** It is declared in
`RefusalKind` and unreachable through the driving ports: every account is opened
in the ledger's single configured currency and `POST /accounts` takes no
currency field. That unreachability is how "multi-currency transactions, out of
scope" is *enforced* rather than merely asserted, so reaching it would mean
adding the API surface DISCUSS excluded. Its coverage sits at layer 1, on the
existing § PBT obligations entry "relax the same-currency assumption" over
`domain.Post`. The `ParseRefusalKind` row carries a comment saying so, in the
same terms DESIGN asked of the production constant.

**Latent wire-value contradiction fixed.** `domain_types.go` declared
`UnknownAccount RefusalKind = "unknown_account"` while
`internal/domain/violation.go` and `journeys/post-a-transfer.yaml` both declared
`account_not_found`. DESIGN settled the wire value as `account_not_found` (two
authoritative sources against one, and the two are the journey the behaviour was
promised in and the site where it is decided) and flagged rather than edited the
mirror. The string literal is corrected; the Go identifier `UnknownAccount`
stays. Nothing else depended on it — the `ParseRefusalKind` row keys off the
Gherkin phrasing "an unknown account", not the wire value, so it needed no
change. This was a real latent failure, not a cosmetic one: two scenarios
asserted a wire value the implementation would never emit, and the RED gate
could not catch it because both die in their `Given` long before the assertion.
It would have surfaced as a mystery red during DELIVER GREEN, against correct
production code.

**The no-new-decorators expectation held on the Then side and did not on the
When side.** DESIGN expected none, on the grounds that one refusal step covers
the whole sealed taxonomy. That is exactly right for the answer: all three new
members are asserted through the existing refusal step, and the only change to
it was widening its subject alternation by one noun (`the account`, alongside
`the transfer`, `the trace`, `the second request`, `the caller`) so that
`Then the account is refused as already open` reads as English — a widened
regex, not a new decorator. The taxonomy itself cost nothing but three constants
and three `ParseRefusalKind` rows, as the type was designed to.

It could not hold on the question. Two new `When` decorators were unavoidable,
and the reason is structural rather than an oversight: **every existing `When`
is typed, and a typed `When` cannot express an input the type system refuses to
hold.** `SubmitTransfer` marshals a `Transfer` through `encoding/json`, so a
body it produces is by construction a body the ledger can parse — it can never
ask what happens to one that is not. `ParseMoney` rejects three decimal places
and anything past int64, correctly, so no existing step can put `50.001` to the
ledger. The two additions are the narrowest that fix this, and both are
parameterised over a domain type rather than written per literal, per Mandate-12:

- `the integrator submits a transfer request <malformation>` over the new
  `MalformedPayload` type — one decorator, seven shapes, adding an eighth adds a
  member and a row.
- `the integrator moves an amount written as "<amount>" from … to … under key …`
  over the new `AmountLiteral` type — the amount exactly as the caller wrote it,
  which is the only honest way to ask about an amount the suite itself could not
  hold.

**Counts after this pass** (stated here, not propagated — a separate
reconciliation pass owns the other `[REF]` sections): **62 scenario blocks / 70
executed** across the same 7 files; 24 `bounded-change` and 38
`unbounded-preservation` tags against 62 blocks, so the tag invariant still
holds; 337 Gherkin step lines over 95 decorators (step-reuse **3.55×**,
informational, up from 3.42×); error coverage 34 of 70 = **49%** under the
existing counting rule, since all three additions are refusals and every one of
the ten executed rows carries `@error`.

**Undefined steps verified, not assumed.** The suite cannot be dry-run — godog
0.14.1 exposes no dry-run mode and this suite's `TestMain` starts a container on
the first `Given` — so the check was done mechanically instead: extract every
registered regex from `steps_ledger_test.go`, compile every `.feature` into
pickles the way godog does (Background prepended, `Scenario Outline` expanded
against its `Examples`, `And`/`But` inheriting the preceding keyword), and match
each step against the registry under godog's own keyword rule
(`suite.go:keywordMatches` — a `Given` decorator never answers a `When` step).
Result: 532 step invocations across 70 pickles, **0 undefined**. Each of the new
steps was then checked for first-match binding and its captured token fed
through the real `ParseRefusalKind` / `ParseMalformedPayload`, since a step that
binds and then panics in its parser is undefined in every way that matters.
`go build ./...`, `go vet ./...` and `gofmt -l` are clean.

**One HIGH reviewer condition on this pass was deferred, not dropped.**
`@nw-acceptance-designer-reviewer` returned CONDITIONALLY_APPROVED on the C2b /
C6a work with one condition: **no HTTP status code is asserted anywhere in the
suite**. It is suite-wide and pre-existing rather than introduced here, but the
three new scenarios are the first whose entire point is a status distinction —
`malformed_request` 400 versus `invalid_amount` 422 differ in nothing else
observable — which is why it surfaced now. DDD-17's status-follows-site rule and
DDR-3's replay-answers-200 ruling both currently have nothing testing them. The
user decided on 2026-08-19 to record it and move to DELIVER rather than run
another authoring round. Full entry, reasoning and the recommended fix shape:
`distill/upstream-issues.md` § R-2 (**OPEN**, owned by DISTILL). **DELIVER
should treat ADR-008's status table as the authority when implementing the
status mapping — the suite will not catch a wrong status.**

**One unrelated fix came along.** `ledger_world.go` was already unformatted at
`516d535` — the `CaptureUniverse` snapshot literal's keys were misaligned — and
CI job 1 gates on `gofmt -l` being empty, so it was a red build waiting for the
first push. `gofmt -w` on the file I was editing anyway; it accounts for the
four-line alignment change in that snapshot literal, which is not mine.

Falsifier-gate telemetry (§7), 3-month window: `(ledger-core, C1, 1, MEDIUM)`
`(ledger-core, C2, 2, HIGH)` `(ledger-core, C3, 4, MEDIUM)`
`(ledger-core, C4, 2, LOW)` `(ledger-core, C5, 0, —)` `(ledger-core, C6, 1, HIGH)`
`(ledger-core, C7, 1, MEDIUM)`. C5 is the only zero-finding category on this
feature — one of the three consecutive zeroes a prune would need.

**Two consequences this audit created.** Both were raised rather than fixed in
the audit pass and were reconciled straight after, on the same day, under a
count-only mandate — no verdict, rule, obligation or reasoning was touched in
any reviewed document.

1. **The counts were stale everywhere the suite is described.** Reconciled
   2026-08-19: § Scenario list with tags, § Adapter coverage, § Driving adapter
   coverage, § Mandate-12 compliance, § Contract shape classification and
   § Wave decisions summary now read 59 blocks / 60 executed, and
   `distill/red-classification.md` states its 53/54 as the counts *as executed*
   and names the six unexecuted additions. The RED gate verdict is untouched and
   remains an unhedged PASS — nothing added can turn a `MISSING_FUNCTIONALITY`
   into a `SETUP_FAILURE`, because none of the six has ever run. Classifying
   them falls to the second gate run DELIVER already owes as obligation 1.
   Figures that are historical rather than stale were deliberately left alone:
   the gate's 53 `MISSING_FUNCTIONALITY`, its 54 container lifecycles, its "5 of
   53 reach their `Then`", and R-1's "all 53 scenario blocks then existing were
   authored untagged". Each records what happened on a date, not what the suite
   holds now.
2. **The headline error ratio is arithmetic, not coverage.** `@error` plus the
   damage-asserting `@env-corrupted` scenarios is 24 of 60 = 40%, still on the
   line. Five of the six additions are boundary and cardinality edges carrying no
   `@error` tag; counted as error/edge the figure is 29 of 60 = 48%. The `@error`
   tag was never a taxonomy of edges and should not be read as one.

---

## Wave: DISTILL / [REF] Registered outcomes

10 rows in `docs/product/outcomes/registry.yaml`, per-typed-contract grain
(D-5). Registry was empty — first feature, no collision check needed.

| ID | Kind | Contract |
|---|---|---|
| OUT-1 | operation | Post a transfer — atomic, at most once per key |
| OUT-2 | operation | Open an account of a declared kind |
| OUT-3 | operation | Report a stored balance |
| OUT-4 | operation | Report ordered entries with a running balance |
| OUT-5 | operation | Answer whether the books balance, with drift attribution |
| OUT-6 | specification | The posting rulebook, as one pure decision |
| OUT-7 | invariant | I1 — entries sum to zero per currency |
| OUT-8 | invariant | I4 — no negative wallet balance |
| OUT-9 | invariant | I7 — repeated application changes state once |
| OUT-10 | invariant | D7 — entries are never updated or deleted |

Written directly rather than via `nwave-ai outcomes register`: that command
fails in this install with `FileNotFoundError` on its own packaged
`docs/product/outcomes/schema.json`. The rows match
`nwave_ai/outcomes/domain/serialization.py:outcome_to_dict` exactly. **The CLI
defect is a tooling bug worth reporting upstream** — it is not a project issue
and nothing here depends on it.

---

## Wave: DISTILL / [REF] Pre-requisites

Environment and upstream artefacts the scenarios depend on. What DELIVER must
have in place before it starts. (Obligations DELIVER must *discharge* are not
pre-requisites and are not listed here — see § Wave decisions summary.)

- **A Docker daemon.** Every scenario reaches for PostgreSQL 16 via
  Testcontainers on its first `Given`, one container per scenario. Verified
  available and sufficient on 2026-08-19: Docker 29.7.2, testcontainers-go
  v0.33.0, `postgres:16`, 54 container lifecycles, no leaks. Gate result and its
  caveats: `distill/red-classification.md`.
- **Two database roles** (`ledgerops_app`, `ledgerops_migrate`) created by the
  migration set. The suite already connects as the app role and reaches for the
  migrate role only to corrupt; neither exists until DELIVER writes migration 0.
- **`golang-migrate` migration set**, expand-only. `MigrationStatements()` is
  scaffolded to return an empty map, so the "no migration erases an entry"
  scenario passes vacuously until real migrations exist. **DELIVER must not
  treat that green as coverage.**
- **`docker-compose.yml` plus `make demo-01..05`, `chaos-01`, `race-02`,
  `race-03`, `corrupt-04`, `trace-05`** — named in the DoD and in every KPI
  contract, not yet written. The KPI harnesses must emit the denominators
  (`iterations`, `submissions`, `injections`), which the scenarios already
  assert.
- **`golangci-lint` with the `exhaustive` linter** over switches on
  `domain.ViolationKind` — the compensating control DESIGN assigned to DEVOPS
  for the hand-built sealed taxonomy.

---

## Wave: DISTILL / [REF] Wave decisions summary

**Scenarios**: 60 across 7 files (59 blocks, one `Scenario Outline` expanding
to 2), 40% `@error` and 48% error-or-edge, one walking skeleton, all but the
skeleton `@pending` for one-at-a-time delivery. Six of the sixty were added by
§ AT completeness audit after this summary was first written; the counts here
are reconciled to the suite on disk, nothing else in this section changed.

**Reconciliation**: passed after three user rulings — DDR-1 (key carried from
slice 01), DDR-2 (console asserted over HTTP, browser E2E deferred), DDR-3
(replay answers 200). All three are recorded in `distill/upstream-issues.md`
with the upstream edits each still owes.

**Tiering**: Tier A only; Tier B skipped because an in-memory composition would
model the transactional behaviour under test, which is the reasoning that
selected strategy C in the first place.

**Constraints established**: `.feature` files are the scenario SSOT and execute
under godog · the suite holds two DSNs and never connects as the migrate role
except to corrupt · the only fakes are `Clock` and `IDGenerator` · every
precondition is established through a driving port, never by writing to the
store · every contended assertion carries its denominator.

**Upstream changes**: six findings raised, **all closed** — DDR-1/2/3 by user
ruling, U-1 (slice-03's untestable "killing the process is not possible"
criterion, owner DISCUSS) and U-2 (KPI-1's `elapsed_ms` guardrail missing the
`corrupted` environment, owner DEVOPS) by their owning wave's reviewer. Each
upstream edit was made only after that reviewer confirmed the finding. The
sixth, R-1, came out of the review cycle itself: all 53 scenario blocks then
existing were
authored without the mandatory `@contract-shape:` tag (principle 14), Sentinel
rejected the wave on it, and all 53 were tagged against an authoritative
DESIGN § Contract shape per component. Detail in `distill/upstream-issues.md`.

**RED gate**: executed 2026-08-19 and **PASSED** — 53 `MISSING_FUNCTIONALITY`,
0 `SETUP_FAILURE`, 0 `BROKEN`, 0 undefined steps, 1 vacuous pass. The gate's
verdict is binary (block on any category-2/3 failure, otherwise pass) and
nothing classified there, so handoff to DELIVER is not blocked. Scenarios were
classified by the *cause* of failure, not the depth reached — which is what
Mandate 7 requires, since scaffolds raise assertion failures precisely so the
snapshot classifies by error type. Under a depth-based rule this gate would read
PARTIAL; that reading and why it is rejected are both stated in
`distill/red-classification.md`.

**Two obligations carried into DELIVER.** These are additional work assigned on
the strength of the PASS, not conditions on it.

1. **A second gate run.** Only 5 of the 53 correct failures reach their `Then`
   today; the other 48 stop in a `Given`, because seeding funds accounts through
   the real `POST /transfers` rather than a back door — Mandate 1 working, not a
   defect, and self-clearing once slice 01 lands. Until then those 48 assertion
   bodies have never executed and a logic error inside one would not have
   surfaced. After slice 01 goes green, re-run the full suite with the tag filter
   disabled and verify the failures have moved from `Given` clauses to `Then`
   clauses. Any scenario still failing in a `Given` at that point is a test
   defect and blocks the slice.
2. **Do not count the vacuous pass.** `The schema builds from nothing` passes
   only because `MigrationStatements()` returns an empty map. It is not coverage
   and must not enter KPI accounting until migration 0 exists.

Also unproven by this run and deferred to DELIVER: every SQL path, the OPS-10
two-role split, row-lock ordering, privilege revocation, and the API-key
middleware — no pgx connection was opened, and all 3 auth scenarios died in
their `Given`.
