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

---

## Wave: DESIGN / [REF] Open questions

Deferred to DISTILL or DELIVER:

- SPA framework selection (React / Vue / Svelte) — DELIVER
- Idempotency key expiry and garbage collection — known gap, no owner yet
- Currency scale handling once a second currency appears — column exists, logic does not
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
(DDD-14) · domain types are immutable with unexported fields (DDD-15).

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
- **DDD-12 exhaustiveness** — `golangci-lint` runs the `exhaustive` linter over
  violation-kind switch sites. This is the compensating control `brief.md:131`
  explicitly assigned to DEVOPS, now discharged
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
