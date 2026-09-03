# Feature Delta — multitenancy

Density: `lean` (Tier-1 `[REF]` only). Expansions available on request via
`--expand <id>`.

This feature reactivates `J6` (`docs/product/jobs.yaml`), dropped from
`ledger-core` scope on 2026-08-17 as D8 ("single-tenant; tenant isolation
deferred... revisit before any third party's money is held"). No DISCOVER or
DIVERGE wave ran for this feature; the deferral's own stated trigger condition
is the evidence that authorizes revisiting it now — recorded as the same kind
of founder-estimated basis `ledger-core`'s own jobs carry, not new user
research.

**Prior Wave Consultation — reading confirmation**

- ✓ `docs/product/jobs.yaml`
- ✓ `docs/product/vision.md`
- ✓ `docs/product/architecture/brief.md`
- ✓ `docs/product/personas/integrating-developer.yaml`
- ✓ `docs/product/personas/platform-operator.yaml`
- ✓ `docs/product/journeys/post-a-transfer.yaml`
- ✓ `docs/product/journeys/verify-the-books.yaml`
- ✓ `docs/product/outcomes/registry.yaml`
- ✓ `docs/product/kpi-contracts.yaml`
- ✓ `docs/feature/ledger-core/feature-delta.md`
- ✓ `docs/feature/ledger-core-console/feature-delta.md` (partial — System
  Constraints, Locked decisions, JTBD sections; full file exceeds a single
  read window, remainder not required for this feature's scope)
- ✓ `internal/domain/account.go`, `internal/adapters/postgres/migrations/0001_init.up.sql`,
  `internal/adapters/http/router.go` — code-level brownfield verification,
  not a documented prior-wave artifact, done because Decision 2 ("Depends")
  requires evaluating existing structure before deciding on a walking skeleton
- ⊘ `docs/feature/multitenancy/discover/` (not found — no DISCOVER wave ran)
- ⊘ `docs/feature/multitenancy/diverge/recommendation.md`,
  `job-analysis.md` (not found — no DIVERGE wave ran)
- ⊘ `docs/project-brief.md`, `docs/stakeholders.yaml` (not found)

**Brownfield finding (Decision 2 resolution)**: zero existing tenant
scaffolding anywhere in the codebase. `internal/domain/account.go` has no
tenant concept. `migrations/0001_init.up.sql` gives `accounts.id` a bare
`PRIMARY KEY` — global uniqueness, not scoped to anything. `router.go`'s
`requireOperatorKey` middleware checks one shared secret (`Deps.OperatorKey`)
for the entire deployment; there is no per-caller identity at all today, let
alone per-tenant. The five shipped slices (01–05) and the console feature
built directly on top of this single-tenant, single-key model. **This feature
cannot slot into existing slices — it needs its own walking skeleton.**

---

## Wave: DISCUSS / [REF] Persona ID

- **P1 — Integrating developer** (`docs/product/personas/integrating-developer.yaml`)
  Now modeled as: one instance of P1 per tenant. Isolation is a property P1
  experiences passively (job J6) — they hold one `tenant_key` and it only ever
  shows them their own accounts.
- **P2 — Platform operator** (`docs/product/personas/platform-operator.yaml`)
  Provisions each tenant and verifies each tenant's books independently (job
  J7). Remains the single real operator; tenants are not additional operators.

No new persona introduced. `vision.md` ("not a customer-facing product") rules
out modeling a tenant's own end-customers as a persona — a tenant is an
organization using ledgerops through P1, not a new user class ledgerops talks
to directly.

---

## Wave: DISCUSS / [REF] JTBD one-liner

Let more than one customer share a ledgerops deployment with no path for one
tenant's data to be read, written, or collided with by another — provable, not
promised.

Full jobs J6 (reactivated) and J7 (new) with dimensions, four forces, and
opportunity scores: `docs/product/jobs.yaml`. Same evidence caveat as J1–J5:
founder-estimated, no user interviews.

---

## Wave: DISCUSS / [REF] Locked decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Feature type: cross-cutting | User-provided (pre-dispatch). Confirmed by brownfield read: domain core, storage, and HTTP API all lack any tenant concept |
| D2 | Walking skeleton: **needed, this feature's own** — not reused from ledger-core | User-provided as "Depends"; resolved after brownfield evaluation. No existing slice touches tenancy; slices 01–05 are already shipped single-tenant |
| D3 | UX research depth: lightweight | User-provided. One journey, happy path plus the one adversarial step that is the feature's reason to exist (`docs/product/journeys/onboard-and-isolate-a-tenant.yaml`) |
| D4 | JTBD mandatory; every story carries a `job_id` | User-provided (nWave default). J6 reactivated on P1, J7 added on P2 |
| D5 | **Scope reduced**: console/SPA changes excluded from this feature | Mirrors the ledger-core → ledger-core-console precedent exactly (backend/API slices shipped first, console carved into its own feature later). Keeps this feature to 3 layers (domain, storage, HTTP API) instead of 4 |
| D6 | Scope Assessment: **oversized as originally framed, resolved by D5 + slicing** | See § Scope Assessment below |
| D7 | Tenant provisioning is operator-driven only; no self-service tenant signup in this feature | Consistent with today's architecture — `platform-operator.yaml` already states "single seeded operator account; no registration, no other users." A signup flow is new surface area this feature does not need to prove isolation |
| D8 | Interim auth model: the existing `OperatorKey` is reused as the platform-admin credential (provisions tenants **and continues to authenticate every existing unscoped console-facing call** — `GET /console/verdict`, `GET /accounts/{id}/entries`, `GET /health/trial-balance`); a new `tenant_key` credential type is scoped to exactly one tenant | Reuses existing auth machinery rather than inventing a three-tier system. **Provisional** — flagged against the tentative `operator-authentication` companion feature already named in `ledger-core-console`'s D10; if that feature ships a real login/session model, this interim decision is expected to be revisited, not treated as final. **Broadened 2026-09-03**: the console-compatibility condition under which the user confirmed D5 (below) means `OperatorKey`'s continued role is not just "admin actions" — it must keep authenticating the exact set of calls the shipped console already makes, unchanged |
| D9 | I8 (tenant isolation) should be enforced by construction, not verified after the fact | Mirrors this codebase's own precedent: I1 and I4 are enforced in the pure domain core and DDD-18 by a database constraint, none are "checked" the way I3's drift-scan checks a derived value. Recorded as a DESIGN pre-requisite, not decided here — DISCUSS does not choose enforcement mechanisms |

---

## Wave: DISCUSS / [REF] Scope Assessment (Elephant Carpaccio Gate)

Run before journey/story-map investment, per Phase 1.5.

| Signal | Value | Oversized? |
|---|---|---|
| >10 user stories | 3 stories (after D5 scope reduction) | No |
| >3 bounded contexts or modules | 1 bounded context (Ledger — tenant is a partition dimension within it, not a new context; **flagged for `nw-ddd-architect` to confirm, not asserted as settled**), but 3 architectural layers (domain, storage, HTTP API) | Borderline — resolved by D5 |
| Walking skeleton requires >5 integration points | Domain core (invariant rescoping) + storage (schema + constraints) + HTTP auth (new credential type) + HTTP handlers (tenant-scoping every existing route) = 4 | No, after console removed |
| Estimated effort >2 weeks | **As originally framed (including console): yes** — retrofitting isolation onto 5 already-shipped slices plus a new credential tier plus console filtering is comparable in size to `ledger-core`'s own original 5-slice DELIVER effort | Yes, originally |
| Multiple independent user outcomes that could ship separately | Yes: (a) tenant identity exists at all, (b) tenant isolation is enforced for accounts/transfers, (c) per-tenant verification, (d) console tenant-awareness | Yes |

**2+ signals fired against the original (console-inclusive) framing → oversized.**
Resolution: D5 removes console scope (deferred to a follow-on feature,
tentatively `tenant-console`, mirroring the `ledger-core-console` precedent
exactly), and the remaining backend/API scope is decomposed into 3
Elephant-Carpaccio slices (below), each ≤1 day.

**Confirmed 2026-09-03 — conditional, not unconditional.** The user's
confirmation ("okay i confirm the deferral") was given against a framing that
made the deferral's condition explicit: the console authenticates today with
the single shared `OperatorKey` and calls `GET /console/verdict`,
`GET /accounts/{id}/entries`, and `GET /health/trial-balance` **unscoped**
(no `tenant_id`); a tenant-scoped API could break that path unless handled
deliberately. The confirmation is of "console work is out of this feature's
build scope," **not** "the console is allowed to break while this feature
ships." That distinction is now a locked cross-cutting constraint — see
§ Acceptance criteria, § Definition of Done, and § Driving ports below, all
updated as a result of this confirmation, not left as an open DESIGN question.

---

## Wave: DISCUSS / [REF] User stories with elevator pitches

### US-1 — Provision a tenant
`job_id: J7` | slice 01

As the platform operator, I create a new tenant and receive a credential
scoped to it, so I can onboard a third party without touching any other
tenant's setup.

**Elevator Pitch**
Before: there is exactly one shared API key for the whole deployment; there is no way to add a second, isolated customer without handing them the same key everyone else already holds.
After: run `curl -X POST /tenants -H 'Authorization: Bearer <platform-admin-key>' -d '{"name":"Acme Wallet"}'` (illustrative — DESIGN settles the exact port) → sees `{"tenant_id":"tnt_1","name":"Acme Wallet","tenant_key":"tk_..."}`
Decision enabled: whether to hand this credential to the new customer and consider them onboarded.

**Domain examples**
1. Happy path — operator provisions "Acme Wallet" → receives `tnt_1` / `tk_acme...`; provisions "Beacon Marketplace" → receives `tnt_2` / `tk_beacon...`, independently.
2. Edge case — operator provisions a second tenant while the first tenant is actively posting transfers; the first tenant's calls are unaffected (bounded-change: only the new tenant record and its credential are written).
3. Error/boundary — operator submits `{"name":"Acme Wallet"}` twice → second call refused `409 tenant_already_exists`, first tenant untouched (mirrors DDD-18/`account_already_exists` precedent).

**UAT Scenarios**
```gherkin
Scenario: Provisioning a tenant issues a scoped credential
  Given the operator holds the platform-admin credential
  When the operator provisions a tenant named "Acme Wallet"
  Then the response includes a tenant_id and a tenant_key
  And the tenant_key is distinct from the platform-admin credential

Scenario: Two tenants can be provisioned independently
  Given "Acme Wallet" has already been provisioned as tnt_1
  When the operator provisions a second tenant named "Beacon Marketplace"
  Then the response includes a new tenant_id (tnt_2) and a new tenant_key
  And tnt_1's tenant_key and data are unchanged

Scenario: A duplicate tenant name is refused
  Given "Acme Wallet" has already been provisioned as tnt_1
  When the operator provisions another tenant named "Acme Wallet"
  Then the response is 409 tenant_already_exists
  And no new tenant is created

Scenario: A non-admin credential cannot provision a tenant
  Given a caller holds a tenant-scoped tenant_key, not the platform-admin credential
  When that caller attempts to provision a new tenant
  Then the response is 401 unidentified_caller
  And no tenant is created
```

**Acceptance Criteria**
- [ ] Provisioning with the platform-admin credential returns a distinct `tenant_id` and `tenant_key`
- [ ] Provisioning two tenants never reuses or collides a `tenant_id` or `tenant_key`
- [ ] A duplicate tenant name is refused `409 tenant_already_exists`, first tenant unaffected
- [ ] A tenant-scoped `tenant_key` cannot call the provisioning endpoint

---

### US-2 — Operate within a tenant, invisible to every other tenant
`job_id: J6` | slice 02 | invariant I8

As an integrating developer, I open accounts and post transfers using my
tenant's credential, and no other tenant's credential can see or touch them.

**Elevator Pitch**
Before: every account name is globally unique and reachable by the one shared key; a second customer sharing the deployment could collide on account names or reach the first customer's balances.
After: run `curl -X POST /accounts -H 'Authorization: Bearer <tk_acme>' -d '{"account_id":"wallet-1","type":"wallet"}'` then the same call with `-H 'Authorization: Bearer <tk_beacon>'` → sees both succeed `201`, same `account_id`, different tenants — where today's single-tenant model would answer the second call `409 account_already_exists`.
Decision enabled: trust that onboarding a second customer cannot corrupt or expose the first customer's ledger.

**Domain examples**
1. Happy path — Acme's `tk_acme` opens `alice` (wallet) and `acme-treasury` (system), funds `alice` from `acme-treasury`, posts a $50 transfer `alice → acme-treasury`-counterparty-flow exactly as today's single-tenant API already works.
2. Edge case — Beacon's `tk_beacon` independently opens an account also named `wallet-1` (or `alice`) → succeeds, because uniqueness is now scoped to `(tenant, account_id)`, not global.
3. Error/boundary — a transfer request names `from: alice` (Acme's account) and `to: bob` (Beacon's account) under `tk_acme` → refused; and `tk_beacon` calling `GET /accounts/alice` (Acme's account) → refused as `account_not_found`, not a distinguishable "forbidden" — matching the project's existing minimal-information-leak posture (ADR-009). **Exact refusal code and cross-tenant-transfer refusal name are open DESIGN questions**, not decided here.

**UAT Scenarios**
```gherkin
Scenario: Two tenants independently reuse the same account name
  Given "Acme Wallet" (tnt_1) holds tenant_key tk_acme
  And "Beacon Marketplace" (tnt_2) holds tenant_key tk_beacon
  When tk_acme opens an account named "wallet-1"
  And tk_beacon opens an account named "wallet-1"
  Then both accounts are created successfully
  And each tenant's "wallet-1" holds an independent balance

Scenario: A tenant's transfer is invisible to every other tenant
  Given tk_acme has opened "alice" (wallet) and funded it via a transfer from "acme-treasury"
  When tk_beacon requests GET /accounts/alice
  Then the response is 404 account_not_found
  And no balance or account metadata for "alice" is disclosed

Scenario: A tenant cannot post a transfer touching another tenant's account
  Given tk_acme holds account "alice"
  And tk_beacon holds account "bob"
  When tk_acme posts a transfer from "alice" to "bob"
  Then the transfer is refused
  And neither alice's nor bob's balance changes

Scenario: Existing single-tenant invariants still hold within one tenant
  Given tk_acme has opened "alice" (wallet) with a balance of $10.00
  When tk_acme posts a transfer of $50.00 from "alice"
  Then the response is 422 insufficient_funds
  And alice's balance remains $10.00

Scenario: A malformed or missing tenant credential is refused before any tenant logic runs
  Given a caller presents no Authorization header
  When that caller posts to /accounts or /transfers
  Then the response is 401 unidentified_caller
  And no account or transfer is created for any tenant
```

**Acceptance Criteria**
- [ ] Account name uniqueness is scoped per tenant, not global (two tenants may both hold an account named `alice`)
- [ ] A tenant's credential can never read or write another tenant's account, transaction, or entry
- [ ] A transfer naming accounts from two different tenants is refused, and neither account's balance changes
- [ ] Every existing single-tenant invariant (I1, I3, I4, I7) continues to hold within one tenant's scope, unchanged in behavior from the caller's point of view

---

### US-3 — Verify one tenant's books without seeing another's
`job_id: J7` | slice 03

As the platform operator, I check whether a specific tenant's books balance,
scoped to that tenant alone.

**Elevator Pitch**
Before: `GET /health/trial-balance` answers for the whole deployment; there is no way to ask the question about one customer's slice of it once more than one customer shares the deployment.
After: run `curl '/health/trial-balance?tenant_id=tnt_1' -H 'Authorization: Bearer <platform-admin-key>'` → sees `{"tenant_id":"tnt_1","verdict":"Books balance: YES", ...}` scoped to only Acme's accounts and entries.
Decision enabled: whether to trust tenant 1's books right now, independent of any other tenant's state.

**Domain examples**
1. Happy path — Acme's books balance; operator checks `tenant_id=tnt_1` → `Books balance: YES`, while Beacon's `tnt_2` is simultaneously mid-transfer or even drifted, with no effect on Acme's answer.
2. Edge case — Beacon (`tnt_2`) has a drifted account; operator checks `tenant_id=tnt_1` (Acme) → still `YES`; checks `tenant_id=tnt_2` → `NO`, naming Beacon's drifted account and delta, exactly as today's single-tenant verdict already names drift (US-4/`ledger-core`).
3. Error/boundary — operator omits `tenant_id` entirely → **open DESIGN question** (see journey `open_questions_for_design`): platform-wide aggregate, or a required-parameter refusal. Not decided here.

**UAT Scenarios**
```gherkin
Scenario: A tenant's clean books report YES independent of another tenant's state
  Given tnt_1 (Acme)'s entries sum to zero
  And tnt_2 (Beacon) has a drifted account
  When the operator checks the trial balance for tenant_id=tnt_1
  Then the response states "Books balance: YES"
  And no mention of tnt_2 or its drift appears in the response

Scenario: A tenant's drift is named without exposing other tenants
  Given tnt_2 (Beacon) has a drifted account "bob"
  When the operator checks the trial balance for tenant_id=tnt_2
  Then the response states "Books balance: NO"
  And the response names "bob" and its delta
  And no account belonging to tnt_1 appears in the response

Scenario: Checking an unknown tenant is refused
  Given no tenant with id "tnt_999" has been provisioned
  When the operator checks the trial balance for tenant_id=tnt_999
  Then the response is 404 tenant_not_found
```

**Acceptance Criteria**
- [ ] A tenant-scoped trial-balance check reports only that tenant's verdict, never aggregating or leaking another tenant's drift
- [ ] Drift attribution (account, delta) within one tenant behaves identically to the existing single-tenant `GET /health/trial-balance` contract
- [ ] Checking an unprovisioned `tenant_id` is refused, not silently answered `YES`

---

**Slice composition gate**: every slice contains at least one user-visible
value story. No slice is `@infrastructure`-only. PASS.

---

## Wave: DISCUSS / [REF] Requirements Completeness

Functional: covered (US-1–US-3, provisioning/isolation/verification).
Non-functional: isolation-as-security-property is the NFR this feature exists
to satisfy (I8); performance/scale NFRs not newly introduced — same
correctness-over-throughput ranking as `brief.md`'s quality attributes.
Business rules: account-name uniqueness rescoped from global to per-tenant
(DDD-18 reinterpretation, flagged for DESIGN); tenant provisioning is
operator-only (D7). Estimated completeness: **> 0.95** — the two genuinely
open items (cross-tenant refusal code shape, un-scoped trial-balance
semantics) are explicitly named as DESIGN questions rather than silently
assumed, which is what keeps them from being completeness gaps.

---

## Wave: DISCUSS / [REF] Acceptance criteria

Embedded per story above and in slice briefs —
`docs/feature/multitenancy/slices/slice-NN-*.md`.

Cross-cutting AC applying to every slice:

- [ ] No cross-tenant request succeeds, under any combination of credential and target
- [ ] Every invariant already proven for the single-tenant case (I1, I3, I4, I7) continues to hold, unchanged, within one tenant's scope
- [ ] Every slice ships with a `make demo-NN` target that runs on a clean clone, consistent with the existing `demo-01`..`demo-05` convention
- [ ] **(added 2026-09-03, revised 2026-09-03 per peer-review iteration 2)** Every acceptance scenario in slices 02/03 touching `GET /console/verdict`, `GET /accounts/{id}/entries`, or `GET /health/trial-balance` includes both a `tenant_key`-scoped variant and an unscoped `OperatorKey` variant asserting byte-identical response shape/status to today's contract — CI-gated, not deferred to manual observation (see § Definition of Done item 3a for why this replaced a manual-KPI-gated wording)

---

## Wave: DISCUSS / [REF] Definition of Done

1. All slice AC checked and passing
2. Cross-tenant isolation proven adversarially (not just absence-of-error on the happy path) for every driving port this feature touches
3. Existing `demo-01`..`demo-05` targets still pass unmodified (regression guard — this feature retrofits invariants onto already-shipped behavior)
3a. **(added 2026-09-03, revised 2026-09-03 per peer-review iteration 2 — original wording gated on non-CI-gated KPIs, corrected)** Every acceptance scenario in slices 02 and 03 that touches `GET /accounts/{id}/entries`, `GET /health/trial-balance`, or `GET /console/verdict` is written twice: once for a `tenant_key`-scoped call (new behavior), once for the existing unscoped `OperatorKey` call (must return byte-identical shape and status to today) — both CI-gated. This operationalizes the console-compatibility constraint at the API-contract level, where it is actually gateable. Verifying the console's *rendered UI* against these contracts (`kpi-contracts.yaml` KPI-C1/C2/C3, which that file itself declares `class: manual`, `fails_build: false`) remains `ledger-core-console`'s own DoD responsibility — not duplicated here as a blocking gate this feature cannot actually enforce
4. `make demo-06`..`demo-08` (or equivalent, per slice) succeeds on a clean clone
5. Peer review passed
6. Refactor pass completed
7. DDD-18's rescoping (global → per-tenant account uniqueness) explicitly confirmed by `nw-ddd-architect`, not silently inherited
8. Slice shipped and demoed within one day
9. Learning hypothesis explicitly confirmed or disproved in writing per slice

---

## Wave: DISCUSS / [REF] Out-of-scope

Console/SPA tenant-awareness (deferred to a follow-on feature, D5) · human
operator login/session/roles (belongs to the tentative `operator-authentication`
companion feature, `ledger-core-console` D10) · self-service tenant signup ·
tenant renaming or offboarding · tenant credential rotation · per-tenant rate
limiting, quotas, or billing/metering · **direct cross-tenant transfer
initiated by a tenant's own credential** (permanently unsupported — this is
the isolation guarantee J6 exists to provide, not a gap; see § Changed
Assumptions) · **platform-mediated inter-tenant settlement via a
clearing-account saga** (needed — see `docs/product/jobs.yaml` J8 —
deliberately deferred to its own follow-on feature that depends on this
feature's isolation slices shipping first; see § Changed Assumptions) ·
multi-currency (inherited out-of-scope from `ledger-core`) · migrating
existing dogfood data into a "default tenant" (a DEVOPS/DELIVER concern once
DESIGN settles the schema shape, not a DISCUSS decision).

---

## Wave: DISCUSS / [REF] WS strategy

**Strategy C — real local resources** (per Mandate 5), same rationale as
`ledger-core`: isolation is a property of the real store's constraint and
query behavior. A fake store would model the very behavior under test.

---

## Wave: DISCUSS / [REF] Driving ports

| Port | Surface | Slice |
|---|---|---|
| `POST /tenants` | HTTP (new) | 01 |
| `POST /accounts` | HTTP (existing, now tenant-scoped) | 02 |
| `POST /transfers` | HTTP (existing, now tenant-scoped + cross-tenant refusal) | 02 |
| `GET /accounts/{id}` | HTTP (existing, now tenant-scoped) | 02 |
| `GET /accounts/{id}/entries` | HTTP — **dual-mode**: tenant-scoped for `tenant_key` callers (new), unscoped for `OperatorKey` callers (existing console behavior, must keep working, CI-gated) — **console-consumed, added 2026-09-03**; was missing from this table entirely, a gap surfaced by the console-compatibility confirmation | 02 |
| `GET /health/trial-balance?tenant_id=` | HTTP (existing, extended) | 03 |
| `GET /console/verdict` | HTTP (existing, unchanged — shares `verdictHandler` with `GET /health/trial-balance`; **must keep answering unscoped `OperatorKey` calls identically to today, added 2026-09-03**) | 03 |

All require either the platform-admin credential (`POST /tenants`; unscoped
`GET /console/verdict` and `GET /health/trial-balance`; also how a
tenant-scoped `GET /health/trial-balance?tenant_id=` check is made, per US-3)
or a `tenant_key` (`POST /accounts`, `POST /transfers`, `GET /accounts/{id}`).
**`GET /accounts/{id}/entries` is dual-mode**, the one port where both
credential types are valid for different reasons: a `tenant_key` sees only
that tenant's entries (new, slice 02); the platform-admin credential's
existing unscoped call keeps working exactly as today (console-consumed,
must not regress). Exact credential-to-port mapping is a DESIGN confirmation,
not a new decision; stated here as the DISCUSS-level intent.

**Hard constraint (2026-09-03, console-compatibility confirmation; tightened
2026-09-03 per peer-review iteration 2)**: the existing unscoped
`OperatorKey` call path to `GET /console/verdict`, `GET /accounts/{id}/entries`,
and `GET /health/trial-balance` must return a successful, unchanged response
— verified by a CI-gated acceptance scenario in the same slice that changes
that endpoint's tenant-scoping behavior (slice 02 for entries, slice 03 for
verdict/trial-balance), checked as part of that slice's own acceptance suite
before the slice ships, not as an aspiration for later in the feature.
Whatever DESIGN decides the unscoped call *means* once tenants exist
(§ Pre-requisites #5), "the unscoped call stops working" is not an available
answer.

---

## Wave: DISCUSS / [REF] Pre-requisites

- No DISCOVER/DIVERGE artifacts exist for this feature — same evidence caveat
  as `ledger-core`'s own jobs
- DESIGN must settle, in order of how load-bearing each is:
  1. **DDD-18 rescoping** — confirm with `nw-ddd-architect` that account-name
     uniqueness moves from a global constraint to `(tenant_id, account_id)`.
     This changes an already-shipped invariant's scope; it is not additive
  2. **I8 enforcement mechanism** — construction-time scoping (every domain
     command and repository query carries a tenant_id; recommended, mirrors
     I1/I4/DDD-18 precedent) vs. verification-time scanning (mirrors I3).
     DISCUSS does not choose this — D9 above records the recommendation and
     the reasoning, not the decision
  3. **Credential mechanism** — how a `tenant_key` is verified and mapped to
     a `tenant_id` at the HTTP boundary; relationship to the existing
     `requireOperatorKey` middleware (D8's provisional recommendation: reuse
     `OperatorKey` as platform-admin, add `tenant_key` as a new type)
  4. **Cross-tenant refusal shape** — 404 (deny existence) vs 403 (confirm
     existence, more informative); journey assumes 404, not settled
  5. **Un-scoped `GET /health/trial-balance` / `GET /console/verdict`
     semantics** — **partially settled 2026-09-03**: the call must keep
     succeeding with a meaningful verdict (never a required-parameter
     refusal) — that half is now a hard constraint, not open (§ Driving
     ports). What remains genuinely open for DESIGN: does it mean
     platform-wide aggregate, or does it resolve to a designated default
     tenant? Backward-compatibility question for the already-shipped
     `ledger-core-console`, which calls this endpoint unscoped today
  6. **Migration shape for existing single-tenant data** — must respect the
     existing expand-only migration discipline (`brief.md` § Deployment
     shape); a "default tenant" backfill strategy is DEVOPS/DELIVER's to
     design once the schema shape above is settled, not DISCUSS's
- **Cross-feature dependency risk**: D8's auth decision is explicitly
  provisional against the tentative `operator-authentication` companion
  feature (`ledger-core-console` feature-delta.md § D10), which does not yet
  exist as a chartered feature. If it ships with a different caller-identity
  model, this feature's credential mechanism may need rework
- DEVOPS receives outcome KPIs only (below)

---

## Wave: DISCUSS / [REF] Outcome KPIs

| KPI | Who | Does what | By how much | Measured by |
|---|---|---|---|---|
| Cross-tenant access refusal | Any tenant's credential | Attempts to read or write another tenant's account, transaction, or entry | Refused 100% of attempts, across the full acceptance suite | CI gate — adversarial cross-tenant scenarios in every slice's acceptance tests |
| Tenant onboarding to first transfer | Platform operator | Provisions a tenant and that tenant posts its first successful transfer | Both complete within a single `make demo-0N` run | CI demo job wall-clock, mirrors KPI-5's pattern |
| Per-tenant trial-balance correctness | Platform operator | Checks one tenant's trial balance while another tenant is drifted or mid-transaction | 100% of CI runs report the checked tenant's own state only, never another tenant's | CI gate — `invariant-gates`-style job, mirrors KPI-1's pattern |

### Metric Hierarchy
- **North Star**: cross-tenant access refusal rate (100% is the only acceptable value — this is a security property, not an optimization target)
- **Leading Indicators**: tenant onboarding completes without touching existing tenants' data (regression guard on D3/US-1)
- **Guardrail Metrics**: existing single-tenant KPIs 1–4 (`kpi-contracts.yaml`) must not regress — this feature retrofits invariants onto already-shipped behavior. **Added 2026-09-03, revised per peer-review iteration 2**: the console-compatibility constraint is enforced as a CI-gated API-contract guarantee (§ Acceptance criteria — unscoped `OperatorKey` calls to the three console-facing endpoints) rather than by gating this feature's DoD on `ledger-core-console`'s own manual, non-CI KPIs (KPI-C1/C2/C3 — `fails_build: false` in `kpi-contracts.yaml`); those remain that feature's own responsibility to self-report, not duplicated as a blocking gate here

**Scope note (2026-09-03, § Changed Assumptions)**: this KPI measures
*tenant-credential-initiated* cross-tenant access only. The future
platform-mediated settlement feature (`jobs.yaml` J8) introduces a distinct,
deliberately-authorized channel that this feature's isolation guarantee does
not — and must not — cover; that channel succeeding is not a violation of
this KPI once it exists, because no tenant credential is the one writing
across the boundary.

---

## Wave: DISCUSS / [REF] DoR Validation

Against the canonical 8-item hard gate (`nw-dor-validation`), same list
`ledger-core` used.

| # | Item | Verdict | Evidence |
|---|---|---|---|
| 1 | Problem statement clear and validated | **PASS** | Domain language, real constraint (D8's own deferral condition), testable — J6, J7 |
| 2 | Persona with specific characteristics | **PARTIAL** | Same inherited caveat as `ledger-core`: P1 is an unvalidated archetype, P2 is real. No new persona introduced, so no new gap beyond the existing one |
| 3 | ≥3 domain examples with real data | **PASS** | Named tenants ("Acme Wallet", "Beacon Marketplace") and accounts ("alice", "bob", "acme-treasury") per story, extending the project's existing dogfood-data convention rather than synthetic placeholders |
| 4 | UAT scenarios cover happy + edge | **PASS** | 4–5 Gherkin scenarios per story, each covering happy path, isolation proof, and at least one refusal |
| 5 | AC derived from UAT | **PASS** | Every AC traces to a scenario above |
| 6 | Story right-sized | **PASS** | 3 stories, one per slice, each ≤1 day, after D5 scope reduction |
| 7 | Technical notes identify constraints | **PASS** | § Pre-requisites names 6 explicit open DESIGN questions rather than silently deciding them |
| 8 | Dependencies resolved or tracked | **PARTIAL** | Slice dependency chain is explicit (01→02→03), but the cross-feature dependency on the not-yet-chartered `operator-authentication` companion feature is a tracked risk, not a resolved one |

**Result: 6 PASS / 2 PARTIAL / 0 FAIL.**

Item 2 is the same inherited, already-waived gap `ledger-core` carries (see
that feature's DoR Waiver) — not a new failure introduced by this feature.
Item 8's PARTIAL is new to this feature and is not waived here: it is a
tracked risk (§ Pre-requisites, Cross-feature dependency risk), acceptable to
carry into DESIGN because the dependency is documented, not because it is
resolved. **DoR passes with one carried-forward waiver (item 2, inherited)
and one explicitly tracked risk (item 8) — not a blocking FAIL.**

---

## Wave: DISCUSS / [REF] Wave decisions summary

**Primary jobs**: isolate tenants so a third party's money can be held
without leaking into another's (J6), onboard and verify a tenant independent
of every other tenant's state (J7).

**Walking skeleton scope**: this feature's own — slice 01 (provision) alone is
not sufficient to prove the feature's reason for existing; the walking
skeleton is slices 01–02 together (provision, then prove a second tenant
cannot touch the first's data), consistent with the journey's S1–S3.

**Feature type**: cross-cutting (domain, storage, HTTP API — console excluded
by D5).

**Constraints established**: tenant provisioning is operator-driven only (D7)
· interim auth model reuses `OperatorKey` as platform-admin, adds `tenant_key`
(D8, provisional) · I8 recommended for construction-time enforcement (D9,
recommendation not decision) · console scope deferred to a follow-on feature
(D5).

**Upstream changes**: J6 reactivated from `docs/product/jobs.yaml`'s
`deferred:` list — see that file's `reactivated:` field on J6 for the full
back-reference. This is not a contradiction of `ledger-core`'s D8
("single-tenant; tenant isolation deferred") — it is that decision's own
named revisit trigger firing.

**Risks flagged for DESIGN/DEVOPS, not resolved here**: DDD-18 rescoping needs
explicit DDD-architect sign-off (§ Pre-requisites #1) · cross-feature
dependency on the not-yet-chartered `operator-authentication` feature
(§ Pre-requisites, Cross-feature dependency risk) · existing-data migration
must respect expand-only discipline (§ Pre-requisites #6) · the Scope
Assessment split (D5/D6) is a recommendation pending the user's explicit
confirmation, not a unilaterally final decision.

---

## Wave: DISCUSS / [REF] Changed Assumptions (2026-09-03 amendment)

In-place amendment, not a restart, made after the first peer-review pass
already approved this feature as originally scoped. Everything below is
additive/clarifying — no story, AC, Gherkin scenario, job story, or slice
brief changed in substance as a result.

**Original wording** (§ Out-of-scope, as first written, before this
amendment): *"cross-tenant transfers (deliberately unsupported, not merely
unbuilt)"* — written without distinguishing *who* is refused from moving
value across a tenant boundary.

**User's correction, verbatim**: "i actually needed cross-tenant transfer but
i didn't specify them so the discuss wave made them out of scope."

**Resolved by the user's direct answers to four clarifying questions Luna
asked before making any change:**

1. **Tenant model — confirmed unchanged.** Tenant = operator company,
   participants = accounts inside it. Cross-tenant settlement is the *rare*
   inter-company case, not routine — tenant-per-participant was explicitly
   rejected: it "would make the boundary-crossing your routine op and kill
   your isolation selling point." Nothing in this feature needed to change
   because of this answer; it confirms the model § Persona ID and J6/J7
   already assumed.
2. **Transaction shape.** Not one transaction spanning the tenant boundary —
   two linked transactions through a platform clearing account, run as a
   saga (initiated → A debited → settled, reversed if the B-leg fails). Each
   tenant's ledger stays independently zero-sum: I1 stays scoped to one
   tenant, and slice 03's per-tenant verification is unaffected.
3. **Authorization.** Platform-mediated settlement authority. The clearing
   account means tenant A's credential only ever touches tenant A's own
   accounts (a debit into A's own clearing account) — no tenant credential
   ever writes into another tenant's ledger. J6's isolation guarantee is
   never violated by the future mechanism; it is architecturally incapable
   of being violated by it. The user noted this extends the existing
   two-role service-credential precedent (`ledger-core` OPS-10:
   `ledgerops_app` vs `ledgerops_migrate`) with a third, platform-settlement-
   scoped credential — recorded here as a breadcrumb for that feature's own
   DESIGN pass, not decided in this one.
4. **Placement.** Its own follow-on feature, after this feature's isolation
   slices ship — not slice 02. "It depends on isolation being finished, and
   folding it in bloats the slice and muddies the race demo."

**What actually changed in this feature's artifacts**: § Out-of-scope now
distinguishes "tenant-credential-initiated cross-tenant transfer" (permanently
unsupported — unchanged in substance, now stated precisely) from
"platform-mediated inter-tenant settlement" (needed, explicitly deferred, not
rejected). § Outcome KPIs gained a scope note on the cross-tenant access
refusal KPI so its future non-conflict with settlement is explicit rather
than assumed. US-2's core scenario — "a transfer naming accounts from two
different tenants, under a tenant's own credential, is refused" — is exactly
what answer 3 confirms must remain permanently true, so it did not change.

**New SSOT breadcrumb**: `docs/product/jobs.yaml` gains a deferred stub
`J8 — Inter-tenant settlement (platform-mediated)`, mirroring the exact shape
`J6` itself had before this feature reactivated it — so the follow-on
settlement feature finds groundwork waiting, the way this feature did.

**Review status**: the changes above are wording/scope-note clarifications
and one new deferred-job stub in SSOT — no AC, Gherkin, DoR item, or slice
composition changed. Self-assessed against `nw-po-review-dimensions` as not
requiring a second full peer-review iteration; offered, not forced, if the
user or DESIGN wants it before handoff.

---

### Console-deferral confirmation, conditional (2026-09-03, same date)

D5's console-scope-exclusion was presented to the user with an explicit
condition attached (paraphrased by the coordinator): console tenant-awareness
deferred to a follow-on feature, **conditional on this feature keeping the
existing console functional throughout** — because the console authenticates
today with the single shared `OperatorKey` and calls
`GET /console/verdict`, `GET /accounts/{id}/entries`, and
`GET /health/trial-balance` unscoped, and a tenant-scoped API could break
that path unless handled deliberately.

**User's confirmation, verbatim**: "okay i confirm the deferral" — given
against that conditional framing.

**This is a real change to the artifacts, not a rubber stamp.** Confirming
"console work is out of this feature's build scope" is not the same as
confirming "the console may break while this feature ships," and the two
were previously conflated: § Pre-requisites #5 had this as an open DESIGN
question with no hard constraint attached, and the Driving ports table was
missing `GET /accounts/{id}/entries` entirely — a real gap this confirmation
surfaced, not a rewording. Both are now corrected:

- § Locked decisions D8 broadened: `OperatorKey`'s continued role covers every
  existing unscoped console-facing call, not only tenant-admin actions
- § Driving ports: added `GET /accounts/{id}/entries` (was missing) and
  `GET /console/verdict`, both marked with the hard constraint that the
  unscoped `OperatorKey` call path must keep succeeding throughout, not just
  by feature completion
- § Pre-requisites #5: the "must not break" half is now locked; only the
  exact unscoped-call *semantics* (platform-wide aggregate vs. a designated
  default tenant) remains open for DESIGN
- § Acceptance criteria and § Definition of Done: gained explicit
  cross-cutting items tying slices 02 and 03 to the console's existing
  manual dogfood checklist (`kpi-contracts.yaml` KPI-C1/C2/C3)
- § Outcome KPIs: KPI-C1/C2/C3 added as guardrail metrics for this feature,
  not only `ledger-core-console`'s own

**Scope impact**: none on slice count or the walking-skeleton definition —
this is a constraint on slices 02 and 03's implementation, not a new slice.
The Scope Assessment (D5/D6) split itself is confirmed and no longer
provisional.

**Peer-review iteration 2 (2026-09-03) — rejected pending revisions, then
remediated.** Because this amendment added real scope (a genuinely missing
driving port, `GET /accounts/{id}/entries`), a second, narrowly-scoped
review pass was dispatched against this delta only. Verdict:
`rejected_pending_revisions` — 1 critical, 2 high, 1 medium, 2 low issues,
all traced to the same root cause: the first draft of this amendment gated
new AC/DoD items on `kpi-contracts.yaml` KPI-C1/C2/C3, which that file
itself declares `class: manual`, `fails_build: false` — unverifiable as
written. Solution-neutrality passed clean; no domain/scope issue was found.

**Remediation applied** (reviewer's recommended Path A): every AC/DoD item
that referenced the manual console KPIs was rewritten to gate on CI-verified
API-contract scenarios instead (unscoped-`OperatorKey` vs. scoped-`tenant_key`
variants of the three console-facing endpoints, asserted byte-identical to
today's contract) — see § Acceptance criteria, § Definition of Done item 3a,
§ Outcome KPIs Guardrail Metrics, § Driving ports, and both slice 02/03
briefs, all revised. Also fixed: the Driving Ports table's credential
sentence and the `GET /accounts/{id}/entries` row now state its dual-mode
behavior explicitly (medium finding); slice 02's effort estimate now names
the dual-mode addition as an explicit, un-absorbed risk rather than silently
assuming it fits ≤1 day (high finding); the "throughout every slice" hard
constraint is now tied to a specific, checkable point — each slice's own
acceptance suite, before that slice ships (high finding).

**This remediation has not itself been re-reviewed** — the review budget for
this wave (max 2 iterations) was spent on iteration 2 above. Per the
product-owner's own methodology, that means escalate rather than dispatch a
third automated pass: the fixes are mechanical, traceable one-to-one to the
reviewer's own stated findings and recommended remedy, and are surfaced here
for the user's or DESIGN's own final look before this is treated as clean.

---

## Wave: DESIGN / [REF] System Architecture

*Owner: nw-system-designer. Full stack DESIGN sequence: system (this
section) -> domain (nw-ddd-architect) -> application (nw-solution-architect).*

**Finding: no new system-level architecture concern.** Verified against this
feature's own artifacts (slice briefs, `onboard-and-isolate-a-tenant.yaml`)
and the brownfield code (`internal/adapters/postgres/migrations/`,
`internal/adapters/http/router.go`), not assumed from the coordinator's
framing. Confirmation written to
`docs/product/architecture/brief.md` § System Architecture / "Multitenancy
(`multitenancy`, confirmed 2026-09-03)", mirroring the `ledger-core-console`
precedent already in that file rather than leaving the section silently
unchanged.

**Back-of-envelope**: tenant count is bounded by operator-driven-only
provisioning (D7) — order of tens to low hundreds, not thousands. Even 100
tenants x 100 accounts = 10,000 account rows, trivial for one PostgreSQL
instance. The existing k6 capacity profile (`tests/load/transfers.js`)
validated 300 rps against this same schema/single-instance shape
(2026-09-02); this feature adds a `tenant_id` filter/key to the same query
shapes without raising aggregate throughput — consistent with this
feature's own § Requirements Completeness ("performance/scale NFRs not
newly introduced"). No headroom concern.

**Confirmed unchanged**: deployment topology (one Go binary, one PostgreSQL
16 instance, no hosted environment); the single-database assumption
(tenancy is a data-partitioning dimension, `tenant_id` as a scoping key —
not sharding); OPS-10's two-role model (new `tenants` table/columns get the
same `GRANT` treatment, no third role — the future J8 settlement feature is
the one place a third role is anticipated, deferred to its own DESIGN);
whole-instance backup/restore (no per-tenant boundary requirement exists);
the composition root's existing wire-then-probe startup check (`cmd/api/`,
OPS-10) already covers the substrate this feature's new table/queries
depend on, so no new probe is introduced.

**Deliberately not addressed, per this feature's own NFR scoping**:
per-tenant connection pooling / bulkhead / noisy-neighbor isolation. I8 is
scoped by DISCUSS as a security/data-isolation property, not a
performance-isolation property — building a bulkhead here would be
speculative infrastructure for a requirement this feature does not carry.

**Handed to the next architects, not decided here**: whether `accounts`'
bare `PRIMARY KEY` becomes a composite `(tenant_id, account_id)` key, how
`entries` carries or derives `tenant_id`, and the expand-only migration
shape for both — schema/domain decisions for `nw-ddd-architect` /
`nw-solution-architect` (§ Pre-requisites #1 and #6).

**Escape hatch, documented not built**: if tenant count or per-tenant
volume grows past the low-hundreds/thousands range estimated above, a
composite index on `(tenant_id, account_id)` is the first lever.

**Open questions for the user**: none at the system/infrastructure level —
this feature's system-level footprint is a confirmed no-op. The feature's
six genuinely open DESIGN questions (§ Pre-requisites) are all
domain/application-level (DDD-18 rescoping, I8 enforcement mechanism,
credential mechanism, refusal shape, unscoped-verdict semantics, migration
shape) and belong to `nw-ddd-architect` / `nw-solution-architect`, next in
this sequence.

---

## Wave: DESIGN / [REF] Domain Model

*Owner: nw-ddd-architect. Full stack DESIGN sequence: system
(nw-system-designer, done) -> **domain (this section)** -> application
(nw-solution-architect).*

**Bounded context: confirmed unchanged.** One context, Ledger — tenant is a
partition dimension within it (§ Scope Assessment's tentative read), not a
new context. No language divergence found across the existing glossary.
Full confirmation: `docs/product/architecture/brief.md` § Domain Model /
"Multitenancy (`multitenancy`, confirmed 2026-09-03)".

**Pre-requisite #1 (DDD-18 rescoping) — resolved.** Account-name/identifier
uniqueness moves from global to `(tenant_id, account_id)`. Renumbered from
the decision-ID `DDD-18` to the invariant-series **I9** (this table's
occupants are otherwise all I-numbered, or D-numbered decisions that read
like one; DDD-18 never fit). Two new invariants added alongside it: **I8**
(tenant isolation, new) and **I10** (tenant-name uniqueness at the Tenant
aggregate, new — structurally identical to I9 one level up). Full table:
`brief.md` § Invariants and where they are enforced.

**Pre-requisite #2 (I8 enforcement mechanism) — resolved: construction-time**,
agreeing with D9's recommendation after evaluating it rather than
rubber-stamping. `tenant_id` becomes a required field on `Account` (no
smart-constructor path omits it), a required repository-query parameter (no
port omits it), and a pre-construction cross-check inside `Post` — the same
gate I1/I4 already occupy, not a query that merely happens to filter
correctly. Rejected: verification-time (I3-style drift scan) — I3's
unenforced status has an independent rationale (the cross-check has
standalone value); I8 has no equivalent reason to stay soft. Full mechanism:
`brief.md` § Multitenancy.

**Tenant modeled as a new aggregate root**, not a value object, not a new
context — owns `{tenant_id, name, credential}`, root-only/value-typed
(Vernon rule 2). Referenced by `tenant_id` from `Account`/`Transaction`
(Vernon rule 3), never embeds them (avoids a three-aggregate-type unit of
work). Bounded-change contracts (declared delta / complement equality) for
Tenant, Account, and Transaction specified in full: `brief.md` §
Multitenancy.

**Pre-requisites #4 and #5 — domain position taken, wire-level mechanics
handed to `nw-solution-architect`:**
- Cross-tenant refusal shape: recommend reusing the sealed
  `account_not_found` kind (no new `ViolationKind` member) — from the
  caller's tenant, the other tenant's account is not merely forbidden, it is
  outside their observable universe. This forces 404 as a consequence of
  existing DDD-17 taxonomy discipline, not a fresh style choice.
- Unscoped trial-balance semantics: recommend "platform-wide aggregate"
  (sum over all entries, unscoped) over "designated default tenant"
  (rejected — no provisioning story, D7 is operator-driven-only). Same
  computation, different entry-set filter; no new aggregate or invariant.

Both are domain-modelling calls, not merely wire-level ones, per the
reasoning in `brief.md` § Multitenancy. Solution-architect's remaining work
is the HTTP-adapter mapping (mechanical given these positions) and
confirming the query-parameter mechanics.

**ES/CQRS: reconfirmed not warranted** for Tenant specifically (all four
heuristic questions answer "no," more decisively than for Account/
Transaction, which already didn't clear the bar under
`adr-003-stored-balances.md`). State-based storage, unchanged.

**Explicitly handed to `nw-solution-architect`, not decided here**:
`tenant_key` credential verification mechanism and its relationship to
`requireOperatorKey` (Pre-requisites #3); the physical `(tenant_id,
account_id)` migration shape, whether `accounts.id` stays a bare column or
becomes composite, and the expand-only mechanics for `entries`/
`transactions` gaining `tenant_id` (Pre-requisites #6, `slice-02`'s
recommended pre-slice SPIKE).

**Open questions for the user**: none — every domain-modelling question this
architect owned (Pre-requisites #1, #2, and the domain half of #4/#5) is
resolved above with evidence-grounded reasoning, not deferred for lack of
information. Nothing here requires the user's judgment call.

Full domain model: `docs/product/architecture/brief.md` § Domain Model.
ADR: `docs/product/architecture/adr-011-tenant-partition-not-context.md`.

---

## Wave: DESIGN / [REF] Application Architecture

*Owner: nw-solution-architect. Full stack DESIGN sequence: system (done) ->
domain (done) -> **application (this section)**.* Full narrative:
`docs/product/architecture/brief.md` § Multitenancy. ADRs:
`adr-012-tenant-credential-mechanism.md`,
`adr-013-multitenancy-migration-shape.md`.

### Decisions

| ID | Decision | Rationale |
|---|---|---|
| DDD-22 | Credential mechanism: `requireOperatorKey` reused unmodified (mounted on `POST /tenants` too); two new middlewares (`requireTenantKey`, `requireTenantKeyOrOperatorKey`) composed from an extracted `bearerToken`/`isOperatorKey` primitive, not copy-pasted. `TenantScope` is a closed two-constructor type (`ScopedToTenant`/`Unscoped`), never a nullable string | D8's structural shape didn't fit `router.go`'s one-middleware-per-group reality; `chi`'s nested groups do. Verified against actual `router.go`, not assumed |
| DDD-23 | **Resolved 2026-09-03 (user decision, relayed by the coordinator, after this DESIGN leg's initial pass had left it open) — Option C.** `POST /accounts`/`POST /transfers`/`GET /accounts/{id}` stay `tenant_key`-only, exactly as DDD-22 originally scoped them; `OperatorKey` gains no tenant-write authority. `demo-01`/`02`/`03`/`chaos-01` keep passing by seeding a second, fixed demo tenant credential (bound to the same `tnt_legacy_seed` tenant the migration already creates for schema reasons, DDD-24) and re-pointing the `Makefile`'s shared `AUTH` variable's *value* to it — every demo/chaos/race recipe body stays byte-for-byte unchanged. **Option A (`OperatorKey` implicitly resolving to the legacy tenant) is rejected.** | User's stated rationale: preserves the admin/tenant-credential boundary DDD-22 already draws (`OperatorKey` = admin actions + unscoped reads, never a tenant's own write authority) — consistent with I8's isolation ethos and this project's correctness/auditability-ranked-first quality attributes. Accepts the cost DDD-22's original framing named: touching a DEVOPS-owned file (`Makefile`) and seeding one more fixed dev secret, precedented by `demo-operator-key` already being one |
| DDD-24 | Migration shape: `accounts`' bare `PRIMARY KEY` becomes composite `(tenant_id, id)`; cascading composite-FK correction on `entries` (`account_id`, `counterparty_id`); plain `tenant_id` column on `transactions`; one expand-only migration, migration-seeded `tnt_legacy_seed` sentinel backfills pre-existing rows | Answers slice-02's SPIKE question directly: yes, expressible as one migration, but with an FK cascade the SPIKE's framing didn't name — `entries`' FKs to `accounts(id)` are structurally invalid once `id` alone stops being unique |
| DDD-25 | Wire mapping: two new sealed `domain.ViolationKind` members, `tenant_already_exists` (409) and `tenant_not_found` (404) — distinct from, not a contradiction of, ADR-011's "no new member for cross-tenant access" (that statement covers I8's reuse of `account_not_found` only; these two are I10's own pair). Cross-tenant `account_not_found` reuse confirmed at the HTTP-adapter mapping layer — no new adapter code path, the existing DDD-17 row already covers a tenant-scoped query returning zero rows | I10 (tenant-name uniqueness) needs the same refusal pair I9 already has for accounts, one aggregate level up — mechanical, symmetric |
| DDD-26 | Reuse Analysis outcome: `requireOperatorKey` EXTENDED (mounted on a new route, zero body change); `TenantKeyResolver`/`requireTenantKey`/`requireTenantKeyOrOperatorKey` CREATE NEW, justified by mechanism difference (static-secret compare vs. keyed lookup), not by "too many dependencies"; `IDGenerator` and `crypto/sha256` EXTENDED (reused as-is, zero new dependency) for credential generation/hashing | Full table: `brief.md` § Application Architecture / Reuse Analysis |

### Reuse Analysis

| Existing Component | File | Overlap | Decision | Justification |
|---|---|---|---|---|
| `requireOperatorKey` middleware | `internal/adapters/http/router.go:144-158` | Admin-gate contract `POST /tenants` needs verbatim; console-facing routes must keep using it unchanged | EXTEND (new route, zero body change) | Cleanest possible reuse — no new code on the path that must stay byte-identical |
| Bearer-header parse/compare inside `requireOperatorKey` | same file | Identical primitive needed by two new middlewares | EXTEND (factor out shared helper) | Reuse at the primitive grain; prevents the three refusal shapes drifting apart |
| `requireTenantKey`, `requireTenantKeyOrOperatorKey` | new | Share the header primitive above | CREATE NEW | Different mechanism (keyed DB lookup vs. static-secret compare), not "too many dependencies" on the existing function — the reverse: extending `requireOperatorKey` would force a dependency its actual callers don't need |
| `TenantRepository`, `ProvisionTenant` | new | No existing tenant persistence anywhere (confirmed by Glob/Grep) | CREATE NEW | Greenfield, mirrors `AccountRepository`/`CreateAccount`'s existing read-decide-write shape |
| `crypto/sha256` | `internal/adapters/http/handlers.go:10` | Already imported in this exact package for fingerprinting | EXTEND (reuse, zero new dependency) | Same class of problem (hash a caller-presented secret) |
| `IDGenerator` port | `cmd/api/main.go:58` | Already `crypto/rand`-backed, 122-bit | EXTEND (reuse for `tenant_id`/`tenant_key` generation) | No new randomness source or port needed |

### Open questions for the user

**None remaining.** This section originally named DDD-23 as the one open
item in this leg — everything else (the credential-mechanism *shape* DDD-22,
the migration shape DDD-24, the wire mapping DDD-25) was already a confirmed,
evidence-grounded decision, verified directly against `router.go`,
`migrations/0001_init.up.sql`, and `Makefile` rather than assumed.

**DDD-23 resolution, additive note (2026-09-03, same DESIGN pass, relayed by
the coordinator after the user reviewed the open question) — not a rewrite
of the record above, the same convention this feature's own DISCUSS-wave
"Changed Assumptions" section used for a late, dated amendment**: the user
chose **Option C**. `OperatorKey` stays strictly admin/unscoped-read (no
tenant-write authority); `demo-01`/`02`/`03`/`chaos-01` are kept passing by
seeding a second, fixed demo tenant credential and re-pointing the
`Makefile`'s shared `AUTH` variable's value to it. Full decision record:
§ Decisions above (DDD-23 row) and `docs/feature/multitenancy/design/wave-decisions.md`.

**DEVOPS inheritance, stated explicitly so it is a requirement DEVOPS
inherits rather than a discovery** — mirrors how DDD-12's `exhaustive`-linter
obligation is handed to DEVOPS elsewhere in `brief.md`: DESIGN's job here
ends at recording the decision. The actual `Makefile` edit (seeding the
second demo tenant + credential, e.g. via a fixed `LEDGEROPS_DEMO_TENANT_KEY`
env var mirroring `LEDGEROPS_OPERATOR_KEY`'s existing pattern, and
re-pointing the `AUTH` variable's value) is a DEVOPS/DELIVER implementation
task, not something this DESIGN wave builds.

### Outcome Collision Check

`nwave-ai outcomes check-delta` was attempted and fails in this install
(known issue — no packaged `schema.json`, per this project's own prior
finding recorded in `docs/product/outcomes/registry.yaml`'s header comment).
Performed by direct reasoning against `docs/product/outcomes/registry.yaml`
instead:

- **New, non-colliding**: `POST /tenants` (`ProvisionTenant`) has no existing
  registry row — genuinely new, artifact `internal/app/usecases.go:Ledger.ProvisionTenant`
  once built. No collision.
- **New, non-colliding**: I8, I9 (rescoped from the pre-existing DDD-18
  concept, never itself registered), and I10 have no existing invariant-kind
  rows (OUT-7/8/9/10 cover I1/I4/I7/D7 only). No collision.
- **Extended, not superseded**: OUT-1 (`PostTransfer`), OUT-2
  (`CreateAccount`), OUT-3 (`GetBalance`), OUT-4 (`GetEntries`), OUT-5
  (`VerifyBooks`) all keep their same `kind: operation`, same `artifact`
  function — this feature only widens their `inputs`/`output` shape
  (tenant-scoped auth, `?tenant_id=` query param, two new refusal codes on
  OUT-1/2/4/5). Flagged for `nw-acceptance-designer`/DISTILL to *update* these
  five rows' shape fields when populating post-delivery, not to register five
  new colliding outcomes or leave the registry silently stale.
- **Assessment: clean.** No feature/registry conflict found; five existing
  rows are flagged for a shape update, not a collision.
