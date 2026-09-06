# Feature Delta — inter-tenant-transfer

Density: `lean` (Tier-1 `[REF]` only). Expansions available on request via `--expand <id>`.

This feature builds job J8 (`docs/product/jobs.yaml`), promoted 2026-09-06
out of `deferred:` where it had sat since multitenancy's DISCUSS wave
(2026-09-03) — the deferral's own stated dependency ("depends on tenant
isolation shipping first") is now satisfied. A prior DISCUSS pass at this
same feature-id explored the *rare, no-pre-established-trust* framing of this
job and was rejected as the wrong scope for what the platform's own
authorization posture should be (locked scope, carried into this session, not
re-litigated here: "enforce only that a tenant pair is pre-authorized — not
personally authorize every transfer"). That prior pass's artifacts are
git-stashed (`stash@{0}`), not part of this feature's lineage, except for two
narrow ideas pulled forward and adapted — see § Salvaged Ideas below.

No DISCOVER or DIVERGE wave ran for this feature. Same evidence basis as
every other job in `jobs.yaml`: founder-estimated, not user-interview-backed.

**Prior Wave Consultation — reading confirmation**

- ✓ `docs/product/jobs.yaml`
- ✓ `docs/product/vision.md`
- ✓ `docs/product/architecture/brief.md` (partial — System Architecture,
  Domain Model through Multitenancy subsection; Application Architecture
  through Reuse Analysis; file exceeds a single read window, remainder not
  required for this feature's DISCUSS-wave scope)
- ✓ `docs/product/personas/integrating-developer.yaml`
- ✓ `docs/product/personas/platform-operator.yaml`
- ✓ `docs/product/journeys/post-a-transfer.yaml`
- ✓ `docs/product/journeys/onboard-and-isolate-a-tenant.yaml`
- ✓ `docs/product/outcomes/registry.yaml` (partial — header + first outcomes; DISTILL-owned artifact, consulted only to confirm no existing contract collision)
- ✓ `docs/feature/multitenancy/feature-delta.md` (partial — header through Locked decisions, Scope Assessment; DISCUSS-wave sections)
- ✓ `docs/architecture/multitenancy/slices/slice-01-provision-a-tenant.md` (style/format precedent)
- ✓ `docs/product/journeys/onboard-and-isolate-a-tenant.yaml` (style/format precedent)
- ✓ `docs/product/architecture/adr-012-tenant-credential-mechanism.md` (credential precedent for the new tenant-link authorization)
- ✓ `internal/domain/post.go` (I8 construction-time check, `TransferCommand` shape — the constraint this feature's 3-leg model must respect)
- ⊘ `docs/feature/inter-tenant-transfer/discover/` (not found — no DISCOVER wave ran)
- ⊘ `docs/feature/inter-tenant-transfer/diverge/recommendation.md`, `job-analysis.md` (not found — no DIVERGE wave ran)
- ⊘ `docs/project-brief.md`, `docs/stakeholders.yaml` (not found)

**Brownfield finding**: no cross-tenant orchestration exists anywhere in the
codebase. `internal/domain/post.go`'s `Post` function refuses any
`TransferCommand` whose snapshots disagree on `TenantID` (I8) — by design,
correctly, and unconditionally. Nothing in this feature changes that refusal;
every leg this feature posts is itself a plain, intra-tenant `Post` call. The
new work is entirely in the application/orchestration layer and two small new
aggregates (tenant-link authorization, counterparty alias) — **this feature
cannot slot into `Post` itself, and does not need to.**

---

## Wave: DISCUSS / [REF] Salvaged Ideas (from rejected prior pass)

Two ideas were pulled forward from the git-stashed, rejected artifact set,
adapted to this feature's own (routine-trust, 3-leg platform-as-hub) account
model — not inherited wholesale, and not the stash's "rare case" framing:

1. **Audit-lookup-by-id.** A single `GET /transfers/{transfer_id}` is the one
   place either tenant or the operator looks to answer "what is the state of
   this transfer?" — carried into journey step S4 and Outcome KPI #3 below,
   and hardened with authorization in slice 05.
2. **Adversarial cross-tenant-isolation proof.** A dedicated scenario proving
   a tenant with no standing link to either party cannot read, resolve into,
   or forge its way into someone else's transfer — carried into journey step
   S7 and slice 05's own acceptance criteria, adapted from a per-account
   isolation proof to a per-transfer, cross-tenant-correlation-id proof
   (the stash's version tested isolation for its own, different, account
   model; the substance — same 404 shape whether forbidden or nonexistent —
   is what transferred, not the shape of the test itself).

---

## Wave: DISCUSS / [REF] Persona ID

- **P1 — Integrating developer** (`docs/product/personas/integrating-developer.yaml`)
  Now also: one tenant's integrator initiating or receiving a transfer
  addressed to/from another tenant's integrator, via an alias, never a raw
  tenant id (jobs J8, J10).
- **P2 — Platform operator** (`docs/product/personas/platform-operator.yaml`)
  Authorizes and revokes standing tenant pairs (job J9). Does not authorize
  individual transfers — that authority belongs to each tenant's own
  `tenant_key`, exactly as I8 already establishes for intra-tenant activity.

No new persona introduced. The counterparty tenant's own integrator is
another instance of P1, not a new persona — symmetric to how `multitenancy`
modeled "one P1 per tenant."

---

## Wave: DISCUSS / [REF] JTBD one-liner

Let two tenants that already trust each other move value directly, by name,
at whatever volume their business needs — without either learning the
other's raw tenant id, and without the platform operator personally
authorizing each transfer.

Full jobs J8 (promoted from `deferred:`), J9 (new), J10 (new) with
dimensions, four forces, and opportunity scores: `docs/product/jobs.yaml`.
Same evidence caveat as every other job: founder-estimated, no user
interviews.

---

## Wave: DISCUSS / [REF] Locked decisions

Carried into this session from a prior DISCUSS pass at this feature-id
(re-verified, not re-litigated) plus this session's own resolutions:

| ID | Decision | Rationale / Status |
|---|---|---|
| D1 | Feature type: **Backend** | Proposed by this agent (no interactive decision tool available in this run) — net-new API + orchestration capability, no new UI surface. **Confirmed by the user, 2026-09-07, as proposed.** |
| D2 | Walking skeleton: **Yes, this feature's own** | Proposed — no existing slice orchestrates a cross-tenant saga; slice 02 (below) is the thinnest end-to-end slice. **Confirmed by the user, 2026-09-07, as proposed.** |
| D3 | UX research depth: **Comprehensive** | Proposed — money crossing a trust boundary, four distinct visible states (pending/settled/retrying/reversed), warrants the same depth as `post-a-transfer.yaml`, not `onboard-and-isolate-a-tenant.yaml`'s lightweight pass. **Confirmed by the user, 2026-09-07, as proposed.** |
| D4 | JTBD: mandatory, every story carries a `job_id` | Default per nWave; applied without needing confirmation — J8/J9/J10 established in `jobs.yaml` |
| D5 | **Scope: routine case only** — tenants have a pre-established trust relationship; either tenant's own key moves value directly, per end-user action, at whatever volume needed. Platform enforces only that a pair is pre-authorized, not each transfer | Carried in, locked. Explicitly not the rare/no-relationship case |
| D6 | **Constraint**: every leg of a transfer is strictly intra-tenant (I8, `internal/domain/post.go:42-53`) — never one `Post` call touching two tenants | Carried in, locked. Re-verified directly against `post.go` during this session, unchanged |
| D7 | **Account model**: 3-leg, platform-as-hub. One settlement account per tenant (not per pair) plus one platform account per tenant — 2N total, not O(N²). Shared `transfer_id` correlates all three legs as one economic event. Settlement/platform accounts are System-kind; balance append-only/derived, never a stored-and-locked column | Carried in, locked |
| D8 | **Saga scheme**: orchestration (per-transfer coordinator, not a shared/global one). Retry-until-settled is the primary path; bounded compensate (reverse completed legs, in reverse order) is a fallback only after a fixed retry count is exhausted | Carried in, locked |
| D9 | **Trust authorization: standing until revoked**, not expiry- or usage-bound. Operator-driven grant/revoke of a tenant-pair link, one decision per pair, not one per transfer | **Resolves open question 1.** Mirrors multitenancy's D7 (no self-service) and ADR-012's rejection of building unrequested machinery ahead of a driving story. No story asks for expiry or usage limits |
| D10 | **End-user visibility: four states — pending, settled, retrying, reversed — readable from one `GET /transfers/{transfer_id}`**, not reconstructed by the caller from three separate ledger lookups | **Resolves open question 2.** See journey `transfer-between-tenants.yaml` steps S3–S6 |
| D11 | **Recipient naming: tenant-scoped `counterparty_alias`**, registered by the sending tenant against an already-active tenant-link, resolving internally to `(tenant_id, account_id)`. Never a raw tenant id on the wire, never a value the receiving tenant must coordinate on | **Resolves open question 3.** See journey step S2 |
| D12 | Scope Assessment: **right-sized, PASS** | See § Scope Assessment below |

---

## Wave: DISCUSS / [REF] Scope Assessment (Elephant Carpaccio Gate)

| Signal | Value | Oversized? |
|---|---|---|
| >10 user stories | 5 stories | No |
| >3 bounded contexts or modules | 1 bounded context (Ledger, per `adr-011-tenant-partition-not-context.md` — this feature adds two small aggregates within it, not a new context) | No |
| Walking skeleton requires >5 integration points | Domain core (no change — reuses `Post` unmodified per leg) + application (new coordinator) + storage (2 new small tables: tenant-link, counterparty alias) + HTTP (3 new routes) = 4 | No |
| Estimated effort >2 weeks | 5 slices × ≤1 day ≈ 1 week | No |
| Multiple independent user outcomes that could ship separately | Partially — authorization (slice 01) is independently valuable/demoable, but transfer/retry/reversal/trace (slices 02–05) form one coherent lifecycle that does not make sense shipped out of order | Borderline, not disqualifying — resolved by carpaccio slicing below, not by feature splitting |

**0–1 signals fire → right-sized.** No feature split required. Proceeds
directly to journey design and story mapping at this scope.

---

## Wave: DISCUSS / [REF] Story Map

**Backbone** (P1/P2 activities, left to right):

| Authorize | Name a counterparty | Send | Observe | Recover |
|---|---|---|---|---|
| Grant tenant-pair link (P2) | Register alias (P1) | Post transfer (P1) | Watch it settle (P1) | See it retry (P1) |
| Revoke link (P2) | | | | See it reverse (P1) |
| | | | | Trace + prove isolation (P1/P2) |

**Walking skeleton**: link authorized → alias registered → transfer sent →
all three legs settle → `GET /transfers/{id}` reports `settled`. This is
slice 02 in full — slice 01 is its own precursor, not folded in, because
authorization is independently demoable and reusable across every future
transfer between that pair.

**Release 1** (outcome: money is never stranded or double-moved under
failure — the reason this feature exists beyond a plumbing exercise): slices
03–04 (retry-until-settled, bounded reversal).

**Release 2** (outcome: no party outside a link can observe or forge into a
transfer): slice 05.

## Priority Rationale

1. **Slice 01 first** — nothing else can be demoed or tested without an
   active link; zero risk, direct precedent (multitenancy slice 01).
2. **Slice 02 second (walking skeleton)** — riskiest assumption first: does
   the 3-leg model actually compose out of three unmodified `Post` calls plus
   a thin coordinator? If not, everything downstream needs rework.
3. **Slice 03 before 04** — retry is the primary path (D8); reversal is a
   fallback that only matters once retry's own state-tracking is proven
   correct.
4. **Slice 04** — highest-uncertainty remaining slice (first
   compensating-transaction logic in this codebase); sequenced right after
   03 so its state dependency is fresh, not deferred to the end where a
   change would ripple back.
5. **Slice 05 last** — hardens authorization on an endpoint that already
   exists functionally since slice 02; needs the full lifecycle (including
   reversal) to have something meaningful to prove isolation against.

---

## Wave: DISCUSS / [REF] User stories with elevator pitches

### US-1 — Authorize a tenant pair
`job_id: J9` | slice 01

As the platform operator, I grant a standing authorization between two
tenants that have agreed to do business, so their integrators can transfer
value to each other without coming back to me for every transfer.

**Elevator Pitch**
Before: no platform-level notion of "these two tenants may trade" exists — every cross-tenant movement is either impossible or would require the operator's involvement every single time.
After: run `curl -X POST /tenant-links -H 'Authorization: Bearer <platform-admin-key>' -d '{"tenant_a":"tnt_acme","tenant_b":"tnt_beacon"}'` (illustrative — DESIGN settles the exact port) → sees `{"link_id":"lnk_1","tenant_a":"tnt_acme","tenant_b":"tnt_beacon","status":"active"}`
Decision enabled: whether to tell both tenants they may now register aliases and begin transferring to each other.

**Domain examples**
1. Happy path — operator authorizes `tnt_acme` ↔ `tnt_beacon` → receives `lnk_1`, `status: active`.
2. Edge case — operator authorizes `tnt_acme` ↔ `tnt_carter` while the `tnt_acme` ↔ `tnt_beacon` link is active and in use; the first link is unaffected.
3. Error/boundary — operator authorizes the same pair twice → second call refused `409 tenant_link_already_exists`; operator revokes `lnk_1`, then re-authorizes the same pair → succeeds with a new `link_id`.

**UAT Scenarios**
```gherkin
Scenario: Authorizing a tenant pair creates a standing link
  Given the operator holds the platform-admin credential
  And tenants "tnt_acme" and "tnt_beacon" both already exist
  When the operator authorizes the pair "tnt_acme" and "tnt_beacon"
  Then a link is created with status "active"
  And the link has no expiry or usage limit

Scenario: A duplicate authorization is refused
  Given tenants "tnt_acme" and "tnt_beacon" already have an active link
  When the operator authorizes the same pair again
  Then the request is refused with 409 "tenant_link_already_exists"
  And the existing link is unaffected

Scenario: Authorizing an unknown tenant is refused
  Given tenant "tnt_acme" exists and tenant "tnt_ghost" does not
  When the operator authorizes the pair "tnt_acme" and "tnt_ghost"
  Then the request is refused with 404 "tenant_not_found"

Scenario: Revoking a link stops future authorization checks from passing
  Given tenants "tnt_acme" and "tnt_beacon" have an active link "lnk_1"
  When the operator revokes "lnk_1"
  Then the link's status becomes "revoked"
  And no new counterparty alias can be registered against "lnk_1"

Scenario: A non-admin credential cannot authorize a pair
  Given a tenant_key belonging to "tnt_acme"
  When that credential attempts to authorize a pair
  Then the request is refused with 401 "unidentified_caller"
```

**Acceptance Criteria**
- [ ] Authorizing an unordered pair returns a `link_id` with `status: active`
- [ ] A duplicate authorization is refused `409 tenant_link_already_exists`; existing link unaffected
- [ ] Naming an unknown tenant is refused `404 tenant_not_found`
- [ ] Revoking a link sets `status: revoked` and blocks new alias registration against it
- [ ] Only the platform-admin credential can authorize or revoke a link

**Outcome KPIs**
- Who: Platform operator (P2)
- Does what: Authorizes a tenant pair for cross-tenant transfer in a single operator action, with no per-transfer involvement afterward
- By how much: 100% of transfers between an authorized pair require zero additional operator action (measured as: operator API calls per transfer between an already-linked pair = 0)
- Measured by: Count of `POST /tenant-links` calls vs. count of `POST /transfers` calls between the same pair, over a rolling window
- Baseline: N/A (capability does not exist today — 0 tenant links, 0 cross-tenant transfers possible)

---

### US-2 — Send a transfer to a named counterparty
`job_id: J8` | slice 02

As a tenant's integrator, I register an alias for a trusted counterparty and
send them a transfer by that alias, so I can move value across the trust
boundary without ever handling their raw tenant id.

**Elevator Pitch**
Before: a tenant's own `tenant_key` can only ever touch its own accounts (I8) — there is no way to move value to another tenant's wallet at all, authorized pair or not.
After: run `curl -X POST /transfers -H 'Authorization: Bearer <tnt_acme tenant_key>' -H 'Idempotency-Key: k1' -d '{"counterparty_alias":"beacon-payout","amount":"50.00"}'` → sees `{"transfer_id":"xfr_1","status":"pending","leg1":{"transaction_id":"txn_1","status":"posted"}}`, and shortly after, `GET /transfers/xfr_1` → sees `{"transfer_id":"xfr_1","status":"settled","legs":[...]}`
Decision enabled: whether to tell the sender's own customer the payment to the counterparty has gone through.

**Domain examples**
1. Happy path — Maria Santos's tenant (`tnt_acme`) sends $50.00 to `beacon-payout` (resolves to `tnt_beacon`'s `wallet-ops` account) → all three legs post, `status: settled`.
2. Edge case — Maria's tenant sends two transfers to the same alias with two different `Idempotency-Key`s in quick succession → two independent `transfer_id`s, each with its own three legs, neither interferes with the other.
3. Error/boundary — Maria's tenant attempts to send to `beacon-payout` after the operator revoked the `tnt_acme`↔`tnt_beacon` link → refused `404 counterparty_not_found`, no leg posts.

**UAT Scenarios**
```gherkin
Scenario: A transfer to a named counterparty settles across three ledgers
  Given "tnt_acme" and "tnt_beacon" have an active tenant-link
  And "tnt_acme" has registered the alias "beacon-payout" resolving to "tnt_beacon"'s "wallet-ops" account
  And "tnt_acme"'s wallet holds $500.00
  When "tnt_acme" sends a transfer of $50.00 to "beacon-payout" with a fresh Idempotency-Key
  Then the response reports status "pending" with leg1 posted
  And "tnt_acme"'s wallet balance becomes $450.00
  When the transfer is queried by its transfer_id
  Then the status is "settled"
  And all three legs (leg1, leg2, leg3) are posted
  And "tnt_beacon"'s "wallet-ops" account balance increases by $50.00

Scenario: Sending to an unregistered alias is refused
  Given "tnt_acme" has not registered any alias named "unknown-partner"
  When "tnt_acme" sends a transfer to "unknown-partner"
  Then the request is refused with 404 "counterparty_not_found"
  And no leg posts

Scenario: Registering an alias against a revoked link is refused
  Given "tnt_acme" and "tnt_beacon" had a link that the operator has since revoked
  When "tnt_acme" registers an alias against that link
  Then the request is refused with 404 "tenant_link_not_found"

Scenario: Insufficient funds refuses before any leg posts
  Given "tnt_acme"'s wallet holds $10.00
  And "tnt_acme" has a registered alias "beacon-payout"
  When "tnt_acme" sends a transfer of $50.00 to "beacon-payout"
  Then the request is refused with 422 "insufficient_funds"
  And "tnt_acme"'s wallet balance remains $10.00
  And no leg posts

Scenario: Retrying the same request is safe
  Given "tnt_acme" has already sent a $50.00 transfer to "beacon-payout" with Idempotency-Key "k1", now settled
  When "tnt_acme" resends the identical request with Idempotency-Key "k1"
  Then the response reports the same transfer_id
  And no new legs are posted
```

**Acceptance Criteria**
- [ ] A happy-path transfer produces three posted legs sharing one `transfer_id`, ending in `status: settled`
- [ ] `GET /transfers/{transfer_id}` reports the transfer's status and all legs
- [ ] Sending to an unregistered or revoked-link alias is refused `404 counterparty_not_found` / `404 tenant_link_not_found`, no leg posts
- [ ] Insufficient sender funds refuses `422 insufficient_funds` before any leg posts
- [ ] Retrying the same `Idempotency-Key` returns the same `transfer_id`, no duplicate legs

**Outcome KPIs**
- Who: Integrating developer (P1), sending tenant
- Does what: Completes a cross-tenant transfer to an already-authorized counterparty in one API call, without handling a raw tenant id
- By how much: 100% of successful transfers require exactly one caller-facing API call (`POST /transfers`) regardless of the three-ledger mechanics behind it
- Measured by: Caller-facing API call count per successful cross-tenant transfer (target: 1)
- Baseline: N/A (capability does not exist today)

---

### US-3 — Watch a stalled leg retry automatically
`job_id: J10` | slice 03

As a sending tenant's integrator, I see a transfer in a `retrying` state
rather than a bare error when a leg stalls, so I know the platform is
handling it and my customer's money is not lost.

**Elevator Pitch**
Before: a transfer spanning three ledgers has no notion of a recoverable, in-progress failure — a failed leg would surface as an undifferentiated error, indistinguishable from a permanent one.
After: run `curl -X GET /transfers/xfr_2 -H 'Authorization: Bearer <tnt_acme tenant_key>'` while leg 2 has failed once → sees `{"transfer_id":"xfr_2","status":"retrying","legs":[{"leg":1,"status":"posted"},{"leg":2,"status":"retrying"},{"leg":3,"status":"pending"}]}`
Decision enabled: whether to tell the customer "still processing" rather than "failed."

**Domain examples**
1. Happy path — leg 2 fails once due to a simulated transient fault, retries, succeeds → transfer reaches `settled` within the retry budget.
2. Edge case — leg 3 (not leg 2) is the one that stalls; leg 1 and leg 2 remain posted and unaffected while leg 3 retries.
3. Error/boundary — leg 2 fails on its first two attempts and succeeds on the third (within a 5-attempt budget) → `status: retrying` is visible during attempts 1–2, `status: settled` after attempt 3.

**UAT Scenarios**
```gherkin
Scenario: A stalled leg 2 retries to settlement
  Given "tnt_acme" has sent a transfer that reaches leg 2
  And leg 2's first attempt fails with a simulated transient fault
  When the transfer is queried immediately after the failed attempt
  Then the status is "retrying"
  And leg 1 remains "posted"
  When leg 2's retry succeeds
  Then the status becomes "settled"

Scenario: A stalled leg 3 does not disturb already-settled legs
  Given "tnt_acme"'s transfer has leg 1 and leg 2 already posted
  And leg 3's first attempt fails with a simulated transient fault
  When the transfer is queried
  Then the status is "retrying"
  And leg 1 and leg 2 remain "posted", unchanged

Scenario: Retries never produce a duplicate posted leg
  Given leg 2 has failed once and is retried successfully
  When the transaction ledger for the platform account is inspected
  Then exactly one posted transaction exists for leg 2 of this transfer_id

Scenario: The sender never sees a bare error while within the retry budget
  Given a transfer is retrying leg 2 within the fixed retry count
  When the sender queries the transfer
  Then the response is 200 with status "retrying", never a 5xx or bare error

Scenario: Funds stay parked, not lost, during retrying
  Given "tnt_acme" sent $50.00 and leg 2 is retrying
  When the platform account's balance is inspected
  Then it reflects exactly $50.00 received from "tnt_acme" via leg 1, pending onward movement
```

**Acceptance Criteria**
- [ ] An injected leg 2 or leg 3 failure results in `status: retrying`, then `status: settled` once the retry succeeds
- [ ] No retry produces a duplicate posted transaction for any leg
- [ ] The sender never receives a bare error response while retries remain within budget
- [ ] Funds are provably present in the platform account throughout retrying

**Outcome KPIs**
- Who: Integrating developer (P1), sending tenant
- Does what: Distinguishes a recoverable in-progress failure from a permanent one, without contacting support
- By how much: 0% of transient leg failures within the retry budget surface as an undifferentiated/bare error to the caller (target: 0)
- Measured by: Count of `retrying`-classified responses vs. count of 5xx/undifferentiated error responses for the same underlying failure class
- Baseline: N/A (capability does not exist today)

---

### US-4 — See a transfer reverse after the retry budget is exhausted
`job_id: J10` | slice 04

As a sending tenant's integrator, I see a transfer reach a terminal
`reversed` state with my own wallet made whole when a leg cannot be
completed, so I never have to wonder whether my money is stuck or gone.

**Elevator Pitch**
Before: retry-until-settled has no defined endpoint — a permanently failing leg would either retry forever or leave the sender's funds parked with no path back.
After: run `curl -X GET /transfers/xfr_3` after the fixed retry budget is exhausted → sees `{"transfer_id":"xfr_3","status":"reversed","reason":"retry_budget_exhausted","legs":[{"leg":1,"status":"reversed"},{"leg":2,"status":"reversed"}]}`, and `GET /accounts/{sender_wallet}` → sees the balance restored to its pre-transfer value
Decision enabled: whether to tell the customer the payment did not go through and safely retry with a new request.

**Domain examples**
1. Happy path — leg 2 fails permanently (past retry budget); leg 1 is reversed; sender's wallet returns to its pre-transfer balance.
2. Edge case — leg 3 fails permanently after leg 2 already posted; both leg 2 and leg 1 are reversed, in that order; the platform account and the sender's wallet both return to their pre-transfer balances.
3. Error/boundary — the sender attempts to resend using the same `Idempotency-Key` after reversal → treated as the original (already-reversed) request, not silently retried as new; a genuinely new attempt requires a new key.

**UAT Scenarios**
```gherkin
Scenario: Exhausting retries on leg 2 reverses leg 1 only
  Given "tnt_acme" sent $50.00 and leg 2 has failed on every attempt within the retry budget
  When the retry budget is exhausted
  Then the transfer's status becomes "reversed" with reason "retry_budget_exhausted"
  And leg 1 is reversed
  And "tnt_acme"'s wallet balance returns to its pre-transfer value

Scenario: Exhausting retries on leg 3 reverses leg 2 then leg 1
  Given leg 1 and leg 2 have already posted and leg 3 fails on every attempt within the retry budget
  When the retry budget is exhausted
  Then leg 2 is reversed, then leg 1 is reversed, in that order
  And "tnt_acme"'s wallet balance and the platform account balance both return to their pre-transfer values

Scenario: Reversal never edits or deletes an existing entry
  Given a transfer has been reversed
  When the entry log for every touched account is inspected
  Then the original legs remain exactly as posted
  And the reversal appears as new, additional entries

Scenario: A reversed transfer is not automatically retried
  Given a transfer has reached status "reversed"
  When time passes with no new caller action
  Then the transfer's status remains "reversed" indefinitely
  And a new transfer requires a new Idempotency-Key

Scenario: A reversed transfer's terminal state names the reason
  Given a transfer has reached status "reversed"
  When the transfer is queried
  Then the response includes reason "retry_budget_exhausted"
```

**Acceptance Criteria**
- [ ] Exhausting the retry budget on leg 2 reverses leg 1 only; sender's wallet restored
- [ ] Exhausting the retry budget on leg 3 reverses leg 2 then leg 1, in order; sender's wallet and platform account both restored
- [ ] `GET /transfers/{transfer_id}` reports `status: reversed` and `reason: retry_budget_exhausted`
- [ ] Reversal adds compensating entries; no existing entry is edited or deleted (D7)
- [ ] A reversed transfer is never auto-retried; a new attempt requires a new `Idempotency-Key`

**Outcome KPIs**
- Who: Integrating developer (P1), sending tenant
- Does what: Recovers full wallet balance automatically when a cross-tenant transfer cannot complete, with no manual reconciliation
- By how much: 100% of permanently-failed transfers (past retry budget) leave the sender's wallet balance identical to its pre-transfer value (target: 100%, zero stranded funds)
- Measured by: Balance-reconciliation check (pre-transfer balance vs. post-reversal balance) run as a property-based test across randomized failure injection points
- Baseline: N/A (capability does not exist today)

---

### US-5 — Trace a transfer, and prove a third tenant cannot forge into it
`job_id: J10` | slice 05

As either tenant party to a transfer, or the platform operator, I look up a
transfer by its id and see every leg's state, while a tenant with no
authorized link to either party gets the same refusal a nonexistent transfer
would produce, so no unauthorized party can observe or affect money that
is not theirs.

**Elevator Pitch**
Before: `GET /transfers/{transfer_id}` (from slice 02) has no hardened authorization boundary of its own — a transfer spans two tenants, and neither existing per-account I8 check nor a naive "any authenticated tenant_key" check is provably correct for it.
After: run `curl -X GET /transfers/xfr_1 -H 'Authorization: Bearer <tnt_carter tenant_key>'` (a tenant with no link to either party) → sees `404 {"error":"transfer_not_found"}` — identical to what a genuinely nonexistent `transfer_id` would return
Decision enabled: whether an auditor or either tenant can trust that a transfer's visibility is bounded to exactly its two parties (and the operator), with nothing to probe.

**Domain examples**
1. Happy path — `tnt_acme` (sender) and `tnt_beacon` (receiver) can each `GET /transfers/xfr_1` and see identical leg detail; the platform-admin credential can too.
2. Edge case — `tnt_carter`, authorized with `tnt_acme` on a *different* link but not with `tnt_beacon`, attempts to read a transfer between `tnt_acme` and `tnt_beacon` → refused, even though `tnt_carter` has some active link elsewhere.
3. Error/boundary — a forged `counterparty_alias` string that happens to collide with an alias registered in a different tenant's namespace resolves to nothing for the forging caller (aliases are tenant-scoped) → `404 counterparty_not_found`, identical shape to a nonexistent alias.

**UAT Scenarios**
```gherkin
Scenario: Either party to a transfer can trace it
  Given a settled transfer "xfr_1" between "tnt_acme" and "tnt_beacon"
  When "tnt_acme" queries "xfr_1"
  Then the response includes all three legs
  When "tnt_beacon" queries "xfr_1"
  Then the response includes the identical all three legs

Scenario: The platform operator can trace any transfer
  Given a settled transfer "xfr_1" between "tnt_acme" and "tnt_beacon"
  When the platform-admin credential queries "xfr_1"
  Then the response includes all three legs

Scenario: A third tenant cannot trace a transfer it is not party to
  Given a settled transfer "xfr_1" between "tnt_acme" and "tnt_beacon"
  And "tnt_carter" has an active link with "tnt_acme" but none with "tnt_beacon"
  When "tnt_carter" queries "xfr_1"
  Then the response is 404 "transfer_not_found"
  And the response is identical in shape to querying a nonexistent transfer_id

Scenario: A forged counterparty_alias resolves to nothing outside its own tenant's namespace
  Given "tnt_beacon" has registered the alias "acme-payout" in its own namespace
  And "tnt_carter" has never registered any alias named "acme-payout"
  When "tnt_carter" sends a transfer to "acme-payout"
  Then the request is refused with 404 "counterparty_not_found"

Scenario: No response distinguishes forbidden from nonexistent
  Given "tnt_carter" queries a transfer_id belonging to two other tenants
  And separately queries a transfer_id that was never issued
  Then both responses are byte-identical in shape (404 "transfer_not_found")
```

**Acceptance Criteria**
- [ ] Either party's own `tenant_key`, or the platform-admin credential, can read a transfer's full detail
- [ ] A tenant party to neither side is refused `404 transfer_not_found`, identical in shape to a nonexistent id
- [ ] A forged or cross-namespace `counterparty_alias` is refused `404 counterparty_not_found`, identical in shape to a genuinely unregistered alias
- [ ] No response distinguishes "exists but forbidden" from "does not exist"

**Outcome KPIs**
- Who: Platform operator (P2) and both tenants party to a transfer (P1 × 2)
- Does what: Traces any transfer's full state from a single lookup, while an unauthorized tenant receives no distinguishable signal of the transfer's existence
- By how much: 0 information-leak incidents (a 403-vs-404 distinguishable response) across every authorization boundary this feature introduces (target: 0)
- Measured by: Adversarial acceptance-test suite (slice 05) run against every combination of {authorized sender, authorized receiver, unauthorized third tenant, platform-admin} × {existing transfer_id, nonexistent transfer_id}
- Baseline: N/A (capability does not exist today)

---

## Wave: DISCUSS / [REF] Definition of Done (DoD)

1. All UAT scenarios across US-1–US-5 pass (green)
2. All supporting tests pass (unit — domain/application; integration — Postgres adapters; component — HTTP)
3. Code refactored, no obvious debt; reviewed and approved
4. Merged to main branch
5. Demoable: an operator can authorize a pair, an integrator can send, retry-recover, and reverse-recover a transfer, and trace it — in one sitting
6. `exhaustive` linter (DDD-12/DDD-17 obligation) covers any new `ViolationKind` members this feature introduces (`tenant_link_not_found`/`tenant_link_already_exists`/`counterparty_not_found`/`transfer_not_found` or equivalent — exact taxonomy is DESIGN's to settle) on both switch surfaces
7. Existing `demo-*`/`chaos-*` Makefile targets pass unmodified (regression guard, mirrors multitenancy slice 01's own AC)
8. D1–D3 above confirmed by the user, 2026-09-07, as proposed — no revision requested; DESIGN proceeds on them as locked

---

## Wave: DISCUSS / [REF] Out-of-scope

- The rare/no-pre-established-trust settlement case (explicitly D5, carried in)
- Per-transfer operator authorization (D5, carried in)
- Bilateral/per-pair account model — O(N²) accounts (D7, carried in — explicitly rejected)
- Choreographed (vs. orchestrated) saga (D8, carried in — explicitly rejected)
- Expiry- or usage-bound tenant-pair authorization (D9, this session)
- Any console/SPA surface for transfers, links, or aliases (mirrors the `ledger-core` → `ledger-core-console` and `multitenancy` → future `tenant-console` precedent: backend first)
- Manual cancel/force-retry controls (slice 03)
- Counterparty tenant notification of a failed/reversed transfer (no story asks for it — the counterparty's own `GET` already answers "did I receive anything")
- Multi-currency transfers (unaffected — `currency_mismatch` remains unreachable through these ports for the same reason it is today)

---

## Wave: DISCUSS / [REF] WS strategy

**A — thin, sequential slices, no configuration switch.** Slice 01 stands
alone (own precursor); slice 02 is the walking skeleton proper; 03–05 extend
it. No environment-switching or feature-flag machinery is needed (unlike WS
strategy D, which would apply if this feature needed to run two settlement
mechanisms side by side — it does not).

---

## Wave: DISCUSS / [REF] Driving ports

| Port | Surface | Slice |
|---|---|---|
| `POST /tenant-links` | HTTP (new, illustrative) | 01 |
| `DELETE /tenant-links/{link_id}` (or equivalent revoke) | HTTP (new, illustrative) | 01 |
| `POST /counterparties` | HTTP (new, illustrative) | 02 |
| `POST /transfers` (extended) | HTTP — new request shape (`counterparty_alias`) alongside the existing intra-tenant shape | 02 |
| `GET /transfers/{transfer_id}` | HTTP (new) | 02 (functional), 03–06 hardened |

All ports illustrative — DESIGN settles exact routes, verbs, and whether
`POST /transfers` grows a discriminated request body or gains a sibling
route. No existing port's byte-identical behavior changes (mirrors the
console-compatibility discipline `multitenancy` established).

---

## Wave: DISCUSS / [REF] Pre-requisites (for DESIGN)

1. **Tenant-link and counterparty-alias aggregate shape** — new aggregate(s)?
   Value objects on Tenant? `nw-ddd-architect`'s to model, following the
   Tenant-as-aggregate-root precedent (`adr-011-tenant-partition-not-context.md`).
2. **Coordinator state persistence shape** — in-process, or a persisted
   per-transfer state row (flagged as a pre-slice spike candidate in slice
   02's brief). `nw-solution-architect`'s to design.
3. **New `ViolationKind` members and their wire mapping** — `tenant_link_not_found`,
   `tenant_link_already_exists`, `counterparty_not_found`, `transfer_not_found`
   (or a smaller/larger set DESIGN judges correct) — both `exhaustive`-linted
   switch surfaces (DDD-12/DDD-17) must grow together.
4. **Whether a new invariant number is warranted** for "leg 2 may only post
   between an authorized tenant pair" (this session names the requirement;
   numbering and enforcement-site choice — construction-time, mirroring I8,
   is this session's *recommendation*, not a locked decision — belongs to
   `nw-ddd-architect`, exactly as I8/I9/I10's numbering was multitenancy
   DESIGN's call, not DISCUSS's).
5. **Synchronous vs. asynchronous settlement** — journey step S3/S4 describe
   the caller's experience as if legs 2/3 may complete within the original
   request-response cycle in the ordinary case, falling back to polling
   `GET /transfers/{id}` only when a leg is retrying. DESIGN confirms whether
   this is achievable or whether `POST /transfers` should always return
   `pending` and require a poll even in the fast path.
6. **Compensating-transaction mechanics under D7** (append-only) — slice 04's
   own recommended pre-slice spike; `nw-ddd-architect` and
   `nw-solution-architect` jointly.

---

## Wave: DISCUSS / [REF] Requirements Completeness

Functional: covered (US-1–US-5, each with 5 UAT scenarios). Non-functional:
no new performance/scale NFR introduced — cross-tenant transfer volume rides
the same headroom analysis `brief.md` § Multitenancy already established (no
story here raises throughput beyond what that analysis covers); security —
covered explicitly by US-5's isolation proof and D9's authorization model;
auditability — covered by D10's four-state visibility and D7's append-only
compensation discipline. Business rules: covered — D5–D11 above, each with
an explicit rationale and rejected-alternative discussion (D9's expiry/usage
rejection). Estimated completeness score: **0.97** (the 0.03 gap is the six
open Pre-requisites items above, each explicitly handed to DESIGN rather than
silently assumed — a named gap, not an unnoticed one).

---

## Wave: DISCUSS / [REF] Definition of Ready (DoR) Validation

| DoR Item | Status | Evidence |
|---|---|---|
| 1. Problem statement clear, domain language | PASS | Every story's narrative is stated in ledger/tenant/transfer domain language, no implementation terms (US-1–US-5 above) |
| 2. User/persona with specific characteristics | PASS (P2 real-project-owner) / PARTIAL (P1 archetype-unvalidated, same caveat as every other P1 job in `jobs.yaml`) | `personas/platform-operator.yaml`, `personas/integrating-developer.yaml` — caveat inherited, not newly introduced by this feature |
| 3. 3+ domain examples with real data | PASS | Each of US-1–US-5 has 3 domain examples; US-2 uses "Maria Santos" as a named individual behind `tnt_acme`, not a generic identifier |
| 4. UAT in Given/When/Then (3-7 scenarios) | PASS | US-1: 5, US-2: 5, US-3: 5, US-4: 5, US-5: 5 |
| 5. AC derived from UAT | PASS | Each story's Acceptance Criteria section maps 1:1 to its UAT scenarios |
| 6. Right-sized (1-3 days, 3-7 scenarios) | PASS | Each slice brief estimates ≤1 day; 5 scenarios each |
| 7. Technical notes: constraints/dependencies | PASS | Slice briefs' Dependencies + Pre-slice SPIKE sections; feature-delta § Pre-requisites |
| 8. Dependencies resolved or tracked | PASS | `multitenancy` (shipped) is the only cross-feature dependency; intra-feature slice dependencies stated in each brief |
| 9. Outcome KPIs defined with measurable targets | PASS | Each story has a Who/Does-what/By-how-much/Measured-by/Baseline block |

### DoR Status: **PASSED**, with one carried caveat (item 2, P1 archetype
status — pre-existing across every P1-driven job in this project, not
specific to this feature). D1–D3 were confirmed by the user, 2026-09-07, as
proposed (see § Locked decisions) — no longer an open condition.

---

## Wave: DISCUSS / [REF] Wave Decisions Summary

# DISCUSS Decisions — inter-tenant-transfer

## Key Decisions
- [D9] Trust authorization is standing until revoked, operator-granted per pair: resolves open question 1 (see `docs/product/journeys/transfer-between-tenants.yaml` § resolved_open_questions)
- [D10] Four visible transfer states (pending/settled/retrying/reversed) via one `GET /transfers/{id}`: resolves open question 2
- [D11] Recipient naming via tenant-scoped `counterparty_alias`, never a raw tenant id: resolves open question 3
- [D12] Scope Assessment PASS — 5 stories, 1 bounded context, ~1 week estimated

## Requirements Summary
- Primary jobs/user needs: J8 (send/receive value across an authorized tenant pair by name), J9 (operator authorizes/revokes the pair), J10 (observe a transfer's exact lifecycle state)
- Walking skeleton scope: slice 02 (authorize → alias → send → all three legs settle → traceable)
- Feature type: Backend (proposed, D1)

## Constraints Established
- Every leg strictly intra-tenant (I8, `post.go:42-53`) — reused unmodified, no domain-core change to the `Post` function itself
- 2N accounts (one settlement + one platform account per tenant), never per-pair
- Retry-until-settled primary, bounded compensate fallback only
- No response may distinguish "forbidden" from "nonexistent" across any new authorization boundary (US-5)

## Upstream Changes
- `docs/product/jobs.yaml`: J8 promoted from `deferred:` to `jobs:` (dependency on `multitenancy` now satisfied); J9, J10 added
- Persona job lists (`integrating-developer.yaml`, `platform-operator.yaml`) extended with J8/J9/J10
- No DISCOVER assumptions changed — none exist for this feature-id

---

## Wave: DISCUSS / [REF] Next Wave

**Handoff To**: `nw-solution-architect` (DESIGN wave, full artifact set) + `nw-platform-architect` (DEVOPS wave, outcome-kpis sections only)
**Deliverables**: this file (`feature-delta.md`) + `docs/feature/inter-tenant-transfer/slices/slice-01..05-*.md` + SSOT updates (`docs/product/jobs.yaml`, `docs/product/journeys/transfer-between-tenants.yaml`, persona files)
**Peer review**: `nw-product-owner-reviewer` — **APPROVED**, verdict recorded 2026-09-07, with user confirmation of D1–D3 as the sole outstanding condition. That condition is now resolved: the user confirmed D1–D3 as proposed on 2026-09-07 (see § Locked decisions, DoD item 8, DoR Validation footer above). No open conditions remain; this feature-delta is ready for DESIGN handoff.
