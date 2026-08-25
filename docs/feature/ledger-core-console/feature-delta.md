# Feature Delta — ledger-core-console

Density: `lean` (Tier-1 `[REF]` only). Expansions available on request via
`--expand <id>`.

This feature is the `web/console/` TypeScript SPA scoped out of `ledger-core`'s
DELIVER run (DDR-2, `docs/feature/ledger-core/distill/upstream-issues.md`).
The backend it renders — `GET /console/verdict`, `GET /accounts/{id}/entries`,
`GET /health/trial-balance` — is already built and demoed
(`docs/feature/ledger-core/feature-delta.md`, § Post-Merge Integration Gate
demo evidence).

**Amendment (2026-08-25)**: the initial DISCUSS pass over this feature used
decision-point answers the orchestrator picked unilaterally instead of
routing to the user. The user's real answers corrected two of them —
Walking Skeleton ("Depends", not "No") and UX research depth
("Comprehensive", not "Lightweight") — and this document has been reworked
in place to reflect them. This is not a from-scratch restart: the DDR-2
constraint, the Elevator Pitch mandate, and the 4 core stories tracing to
J4/J5 all remain valid and are preserved. What changed: Phase 1.5 (Scope
Assessment) now includes an explicit Feature-0 Walking Skeleton evaluation
(previously asserted without reasoning); Phase 2 (Journey Design) was rerun
as a genuine comprehensive pass specific to the console — see
`docs/feature/ledger-core-console/discuss/journey-console-visual.md`,
`journey-console.yaml`, and `shared-artifacts-registry.md` — surfacing two
new console-specific states (loading/freshness, and a second failure branch
on the entries fetch) that fed back into US-1 and US-3; and a new candidate
(account/entry search) was evaluated against the Elephant Carpaccio taste
tests and explicitly deferred rather than silently omitted. Full diff is
recorded in § Amendment Log at the end of this document.

---

## Wave: DISCUSS / [REF] Persona ID

- **P2 — Platform operator** (`docs/product/personas/platform-operator.yaml`)
  The ledgerops project owner, dogfooding as the sole operator. Real person,
  real accountability — not an archetype. Single seeded operator account, no
  registration, no other users.

No other persona touches this feature. P1 (integrating developer) is
API-only and never opens the console.

---

## Wave: DISCUSS / [REF] JTBD one-liner

Let the operator answer "does this add up?" from a browser, in words, before
any figures — and if not, let them find and explain the drift without
touching a terminal.

Reuses validated jobs J4 and J5 from the `ledger-core` DISCUSS wave
(`docs/product/jobs.yaml`) rather than discovering new ones. Both already
carry functional/emotional/social dimensions and four-forces analysis;
those are not re-derived here.

---

## Wave: DISCUSS / [REF] System Constraints

Cross-cutting constraints that shape every story in this feature:

- **DDR-2 (permanent)** — the verdict/drift/trace contract is already
  specified and tested at `tests/acceptance/ledgercore/milestone-04-proof-of-balance.feature`
  and `milestone-05-entry-traceability.feature`, asserted over HTTP/JSON, not
  through a real browser. No AC or UAT scenario in this feature proposes a
  new browser-level `.feature` file or contradicts that ruling. The SPA's
  rendering stays a documented untested seam
  (`docs/architecture/atdd-infrastructure-policy.md` § Known gap) until a
  future feature explicitly reopens it.
- **ADR-006** — the console is a separate TypeScript SPA under `web/console/`,
  not server-rendered. Framework choice (React/Vue/Svelte) is explicitly
  deferred to DESIGN/DELIVER; this feature stays framework-neutral throughout.
- **No new backend work.** Every endpoint this feature consumes
  (`GET /console/verdict`, `GET /accounts/{id}/entries`,
  `GET /health/trial-balance`) already exists and is already under contract
  test. Stories here are pure rendering + fetch wiring.
- **Auth reuse (sharpened, D10).** The console authenticates with the same
  seeded operator API key as every other surface — no login UI, no session
  management, this pass. How the key reaches the browser under that
  static-key model (env-baked build vs. proxy-injected header) is an open
  DESIGN question (see Pre-requisites); a login prompt is explicitly not a
  DESIGN option here — that belongs to the companion `operator-authentication`
  feature.

---

## Wave: DISCUSS / [REF] Locked decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Feature type: user-facing | Orchestrator Decision 1 |
| D2 | **Revised**: Walking Skeleton = "Depends" — evaluated, and slice 01/US-1 already serves as this feature's Feature-0 WS; no separate WS slice added | Corrected Orchestrator Decision 2 — see § Walking Skeleton (Feature-0) Evaluation for the reasoning the prior "No" answer skipped |
| D3 | **Revised**: UX research depth = comprehensive — Phase 2 rerun as a genuine console-specific pass, not a formalization of `verify-the-books.yaml` | Corrected Orchestrator Decision 3 — see `discuss/journey-console-visual.md`, `discuss/journey-console.yaml`, `discuss/shared-artifacts-registry.md` |
| D4 | JTBD mandatory, bridged directly to J4/J5 | Orchestrator Decision 4 (unchanged) — no fresh Job Discovery/Four Forces/Opportunity Scoring; those already exist and are validated in `docs/product/jobs.yaml` |
| D5 | Journey reused, not forked, but extended | `verify-the-books.yaml` S1–S4 still describe the console's happy path and core arc correctly; the comprehensive redo adds console-specific states in a companion `discuss/journey-console.yaml`, not a competing SSOT journey |
| D6 | Scope right-sized, no split | Phase 1.5 gate — see § Scope Assessment (re-run) |
| D7 | Domain examples use real dogfood data, not synthetic | Unlike `ledger-core`'s DoR item 3 (FAIL, waived, D6 there), this feature can reuse `alice-demo` / `bob` / `treasury-demo` from the actual Post-Merge Integration Gate demo (`docs/feature/ledger-core/feature-delta.md` §1732–1736) — real production-shaped data exists this time |
| D8 | Framework choice stays open | ADR-006 defers React/Vue/Svelte to DELIVER; no story or AC in this feature names a framework |
| D9 | **New**: Search/filter over accounts or entries — evaluated, deferred (not added) | See § Search / Filter Candidate — Evaluated and Deferred; fails the "disproves a pre-commitment" taste test and is materially redundant with US-2 |
| D10 | **New**: Auth/authz (login, sessions, roles) stays a separate companion feature, tentatively `operator-authentication` | Corrected Orchestrator Decision 5 (unchanged from prior pass, now stated explicitly) — see § Out-of-scope and § Pre-requisites |

---

## Wave: DISCUSS / [REF] JTBD-to-Story Bridge

| Job | Story | Dimension emphasized |
|---|---|---|
| J4 — "see that the books balance" | US-1, US-2, US-4 | functional: verify at a glance; emotional: sleep without wondering |
| J5 — "trace to the exact entries" | US-3 | functional: drill from balance to entries; emotional: replace suspicion with an answer |

Every story below traces to J4 or J5. No new job is introduced.

---

## Wave: DISCUSS / [REF] Scope Assessment (Elephant Carpaccio Gate)

Run before journey/story-map investment, per Phase 1.5.

| Signal | Value | Oversized? |
|---|---|---|
| User stories | 4 | No (threshold >10) |
| Bounded contexts / modules touched | 1 (`web/console/` only — no backend change) | No (threshold >3) |
| Integration points | 3 (`/console/verdict`, `/accounts/{id}/entries`, `/health/trial-balance`), all pre-existing and already contract-tested | No (threshold >5) |
| Estimated effort | ~3–4 days total across 4 slices | No (threshold >2 weeks) |
| Independent shippable outcomes | 1 coherent outcome (a working console), not several unrelated ones | No |

**Verdict: PASS — 4 stories, 1 bounded context, estimated 3–4 days.** No
split proposed. (Scenario counts within slices grew slightly under the
comprehensive Phase 2 redo — see § User stories — but not enough to change
this verdict: still 4 slices, 1 context, ≤1 day each.)

---

## Wave: DISCUSS / [REF] Walking Skeleton (Feature-0) Evaluation

Per the corrected orchestrator Decision 2 ("Depends" — brownfield; Luna
evaluates existing structure first, rather than defaulting to skip), this
section documents the evaluation the prior pass's flat "No" skipped.

**Brownfield check, done properly**: the `ledger-core` backend (verdict,
drift, entry-trace endpoints) is delivered, running, and already
contract-tested — genuinely brownfield, stable. But `web/console/` itself
does not exist yet: no prior frontend code, no build toolchain, no
dev-server/proxy setup, no framework selection (ADR-006 defers this). From
the SPA's own vantage point this is a **greenfield seam inside a brownfield
service** — exactly the situation Decision 2 exists to catch, not a clean
either/or, which is why "Depends" was the right answer and "No" was not.

**What a Feature-0 WS would retire**: the risk that the full stack doesn't
work end-to-end at all before features get built on top of it. That risk is
already the literal content of slice 01 / US-1: stand up the SPA shell,
wire the build toolchain, fetch one real JSON response from a live
endpoint, render it truthfully. Slice 01's own learning hypothesis
(`docs/feature/ledger-core-console/slices/slice-01-console-states-the-verdict.md`)
is written as a walking-skeleton hypothesis — "disproves the existing
`GET /console/verdict` JSON contract is sufficient to drive a truthful
browser rendering with no additional backend work" — and that brief
independently flags toolchain scaffolding, not rendering logic, as the
likeliest one-day-ceiling breaker, which is precisely the class of risk a
Feature-0 WS exists to surface early.

**Conclusion: no separate Feature-0 walking-skeleton slice is proposed.**
Adding one would duplicate slice 01 almost exactly (same toolchain
stand-up, same first fetch, same rendering-a-string risk) while producing a
second throwaway-or-merge-worthy artifact — which the Elephant Carpaccio
taste tests explicitly flag as decoration ("if 2+ slices are identical
except for scale → merge them"). **Slice 01 / US-1 serves double duty**:
it is both the story-map-level walking skeleton (Phase 2.5, unchanged from
the prior pass) and this feature's Feature-0 WS (Decision 2, newly
evaluated here). This closes the gap in the prior pass, which asserted "no
WS" without checking whether slice 01 already satisfied the Feature-0
concern.

---

## Wave: DISCUSS / [REF] Story Map

**Persona**: P2 · **Goal**: Answer "does this add up?" from a browser, and
explain any drift without a terminal.

Backbone mirrors journey `verify-the-books.yaml` steps S1–S4 directly:

| Open the console (S1) | Read the verdict (S2) | Spot a drifted account (S3) | Trace to entries (S4) |
|---|---|---|---|
| Fetch verdict on page load | State "Books balance: YES/NO" before any figures | List `account_id, stored, computed, delta` for each drift | Click through to ordered entries + running balance |
| **US-1** | **US-1** | **US-2** | **US-3** |
| Fallback on fetch failure — **US-4** | | | |

### Walking skeleton

**US-1** (open console → fetch verdict → state it in words) is the minimum
end-to-end slice: it touches the browser, the fetch, and the existing JSON
contract, and alone answers the operator's first and most important
question. This is both the story-map-level walking skeleton (Phase 2.5) and
this feature's Feature-0 walking skeleton (Decision 2, evaluated explicitly
this pass) — see § Walking Skeleton (Feature-0) Evaluation above for the
full reasoning. It is one slice serving both purposes, not two separate WS
concepts in tension.

### Release slicing

All four stories ship as one release — the feature has no natural second
release, since a console that only states the verdict (US-1) without ever
being able to explain a NO (US-2/US-3) is not yet dogfoodable for J5, and a
console with no fallback (US-4) leaves the error path from `verify-the-books.yaml`
undocumented in the product. All four are Must Have.

### Priority Rationale

1. **US-1** first — walking skeleton, riskiest assumption ("can the SPA
   render the JSON verdict as a truthful sentence at all?"), and the highest
   standalone value (J4's core ask).
2. **US-2** second — depends on US-1's page shell; closes the gap between
   "NO" and "why", which is the point where the parent feature's demo
   evidence stopped (curl only).
3. **US-3** third — depends on US-2's drift list providing the `account_id`
   to click through; delivers J5 in full.
4. **US-4** last by dependency (needs the fetch call US-1 introduces to have
   something to fail), but flagged highest-*risk* of the four — it is the
   only story with genuine design ambiguity (see Learning hypothesis in its
   slice brief) and should not be deferred past this release just because it
   ships last.

---

## Wave: DISCUSS / [REF] Search / Filter Candidate — Evaluated and Deferred

Raised by the user as a possible feature: search/filter over accounts or
entries in the console. Evaluated here explicitly, per Phase 2.5's Elephant
Carpaccio taste tests, rather than silently omitted.

| Taste test | Result |
|---|---|
| Ships 4+ new components? | No — a single client-side filter over already-rendered drift/entries data. Passes thinness. |
| Depends on a new abstraction shipped elsewhere first? | No new abstraction required. |
| Does it disprove any pre-commitment? | **No.** Nothing about this feature's actual risky assumptions (toolchain viability, JSON→UI faithfulness, fetch-failure handling) is tested by adding search. It retires no risk — it is a pure convenience enhancement. Per the taste test, a slice with no falsifiable learning hypothesis is "decoration, not discipline." |
| Uses only synthetic data? | N/A — would use real data, but see redundancy point below. |
| Identical to an existing slice except scale? | **Materially redundant with US-2.** US-2 already answers "which accounts have a problem" by rendering the backend's own `drifted` array — that IS a search/filter, just pre-computed server-side rather than operator-typed. A manual search box would let the operator ask a question the console already answers unprompted. |

Additional grounding checks: neither J4 nor J5 in `docs/product/jobs.yaml`
mention searching or browsing — both are framed as "see that it balances"
and "trace to entries," not "find an account." The persona file
(`docs/product/personas/platform-operator.yaml`) mental model and
frustrations sections contain no complaint about locating accounts. Current
known account volume (`alice-demo`, `bob`, `treasury-demo` — single-digit,
dogfood-scale) does not create a scannability problem the drift table or an
individual account's entry list would need search to solve.

**Recommendation: DEFER, not reject outright.** Revisit if/when either
becomes true: (a) the operator's real account count grows large enough that
the drift table or an account's entry list becomes hard to scan by eye (no
evidence of this yet), or (b) a genuinely new job emerges — e.g., an
auditor or second operator persona wanting to look up an arbitrary
account's history without a known drift — which is a different job than
J4/J5 and would need its own JTBD pass, not a retrofit onto this feature's
stories. No slice, story, or AC is added for search in this release.

---

## Wave: DISCUSS / [REF] User stories with elevator pitches

### US-1 — Console states the verdict
`job_id: J4` | slice 01

As the platform operator, I open the console and immediately know whether the
books balance, in words, before any figures.

**Elevator Pitch**
Before: I have no way to see whether the books balance without opening a terminal and running curl against a health endpoint.
After: open `http://localhost:8080/console` → sees "Books balance: YES" as the first sentence on the page (or "Books balance: NO" when the ledger doesn't balance), stated before any table or number.
Decision enabled: whether to trust the ledger right now, or start investigating drift.

**UAT Scenarios (BDD)**

```gherkin
Scenario: A healthy ledger's console states the verdict in words first
  Given the ledger holds treasury-demo and alice-demo, both settled and consistent
  When the operator opens the console
  Then the page's first sentence reads "Books balance: YES"
  And the sentence appears before any per-account figures

Scenario: An empty ledger still gives a clean answer
  Given the ledger holds no transfers
  When the operator opens the console
  Then the page's first sentence reads "Books balance: YES"

Scenario: A corrupted ledger's console states the verdict in words first
  Given alice-demo's recorded entry amount has been altered by 5.00 out of band
  When the operator opens the console
  Then the page's first sentence reads "Books balance: NO"
  And the sentence appears before any per-account figures

Scenario: The console and the health check agree
  Given alice-demo's recorded entry amount has been altered by 5.00 out of band
  When the operator compares the console's verdict to GET /health/trial-balance
  Then both report "Books balance: NO"

Scenario: The verdict sentence is never buried below a table
  Given any ledger state, healthy or drifted
  When the operator opens the console
  Then no table or per-account figure appears above the verdict sentence

Scenario: The operator sees a checking state while loading, then a freshness label
  Given the console page has loaded and GET /console/verdict has not yet resolved
  When the operator looks at the page during that window
  Then the page shows "Checking the books..." rather than a blank page
  And once the verdict appears, it is labeled with how long ago it was fetched (client-side, not from the server)
```

> **NEW this pass** (comprehensive Phase 2 redo — see
> `docs/feature/ledger-core-console/discuss/journey-console-visual.md`,
> "S1a — Loading" and "S2 — Verdict, now with a freshness label"): a curl
> command has no in-between "loading" state to design; a browser page does,
> and mental-model interrogation found it was undesigned in the prior pass.

**Acceptance Criteria**
- [ ] Console page fetches `GET /console/verdict` on load and renders its
      `verdict` field as a plain sentence, "Books balance: YES" / "Books
      balance: NO", above any other content
- [ ] Verdict sentence renders identically for a healthy, an empty, and a
      corrupted ledger, matching the JSON `verdict` field exactly
- [ ] While the fetch is outstanding, the page shows a neutral "Checking the
      books..." state, never a blank page (new this pass)
- [ ] Once the verdict renders, it is labeled with `${fetched_at}`, a
      client-observed timestamp (browser clock at fetch resolution) — not a
      server field, since `GET /console/verdict` has none today; see
      `discuss/shared-artifacts-registry.md` (new this pass)
- [ ] No AC in this story requires a browser-level test to verify — the
      underlying contract is asserted by `milestone-04-proof-of-balance.feature`
      ("A healthy ledger states its verdict in words before any figures",
      "The console and the health check give the operator the same answer");
      the SPA's faithful rendering of that contract, including the loading
      and freshness states, is a manual dogfood check per DDR-2, not a new
      automated browser test

### US-2 — Console names the drifted accounts
`job_id: J4` | slice 02

As the platform operator, when the verdict is NO, I see which accounts
drifted and by how much, without leaving the console.

**Elevator Pitch**
Before: when the verdict is NO, I must curl `/console/verdict` by hand and read raw JSON to find which account drifted.
After: open `http://localhost:8080/console` while alice-demo's entry is corrupted → sees a table below the verdict listing `alice-demo` with its stored balance, computed balance, and a delta of `5.00`.
Decision enabled: which account to investigate first, and how large the discrepancy is.

**UAT Scenarios (BDD)**

```gherkin
Scenario: A single drifted account is named with its numbers
  Given alice-demo's recorded entry amount has been altered by 5.00 out of band
  When the operator views the console
  Then a table below the verdict lists "alice-demo"
  And that row shows the stored balance, the computed balance, and a delta of 5.00

Scenario: Healthy accounts are not swept into the drift table
  Given only alice-demo is drifted and bob is not
  When the operator views the console
  Then "bob" does not appear in the drift table

Scenario: Two drifted accounts are both named
  Given alice-demo's recorded entry amount has been altered by 5.00 out of band
  And bob's stored balance has been altered by 7.00 out of band
  When the operator views the console
  Then the drift table lists both "alice-demo" and "bob"
  And each row shows its own stored balance, computed balance, and delta

Scenario: A healthy ledger shows no drift table
  Given the ledger is healthy
  When the operator views the console
  Then no drift table is shown, consistent with the verdict-first design
```

**Acceptance Criteria**
- [ ] When `GET /console/verdict`'s `drifted` array is non-empty, the
      console renders one row per entry with `account_id`, `stored`,
      `computed`, and `delta` fields
- [ ] When `drifted` is empty, no drift table is rendered
- [ ] Drift table always renders below the verdict sentence, never above it
- [ ] Underlying data asserted by `milestone-04-proof-of-balance.feature`
      ("A tampered entry turns the verdict red and names the account",
      "Two damaged accounts are both named, not just the first one found",
      "Healthy accounts are not swept up with the drifted one"); the SPA's
      table rendering is the documented untested seam per DDR-2

### US-3 — Console traces a drifted account to its entries
`job_id: J5` | slice 03

As the platform operator, I trace any balance to the entries that produced
it, without leaving the console.

**Elevator Pitch**
Before: I must separately curl `/accounts/{id}/entries` and manually correlate rows to find where a balance diverges.
After: on the console, click `alice-demo`'s row in the drift table → sees `alice-demo`'s entries listed in order with a running balance column, diverging visibly at the altered entry's row.
Decision enabled: which specific entry caused the discrepancy, so the operator can explain what actually happened.

**UAT Scenarios (BDD)**

```gherkin
Scenario: Clicking a drifted account shows its entries
  Given "alice-demo" is listed as drifted on the console
  When the operator clicks alice-demo's row
  Then the console shows alice-demo's entries, ordered, with a running balance column

Scenario: The divergence is visible at the offending row
  Given alice-demo's entries include one altered by 5.00 out of band
  When the operator views alice-demo's entry trace
  Then the running balance diverges visibly starting at the altered entry's row

Scenario: Each entry shows enough to explain it
  Given alice-demo was funded by treasury-demo
  When the operator views alice-demo's entry trace
  Then each entry shows its counterparty account and recorded_at timestamp,
    alongside its amount and running balance

Scenario: A healthy account's trace shows no divergence
  Given bob is not listed as drifted
  When the operator opens bob's entry trace
  Then the running balance matches bob's stored balance at every row

Scenario: A failed entries fetch does not strand the operator mid-investigation
  Given the operator has clicked alice-demo's drift-table row
  When GET /accounts/alice-demo/entries fails or times out
  Then the console shows a bounded-time error naming GET /accounts/alice-demo/entries as the direct fallback
  And the operator is not left with a blank panel or an unending spinner
```

> **NEW this pass** (comprehensive Phase 2 redo — see
> `docs/feature/ledger-core-console/discuss/journey-console-visual.md`,
> "S4-fail"): US-4 only ever handled the *first* fetch (verdict) failing.
> This entries-fetch failure strikes at the peak-tension moment of the
> journey — after the operator is already alarmed by a NO verdict and has
> narrowed to a specific suspect account — making it a materially different
> and higher-stakes failure than the one US-4 already covers. Added as a
> scenario on this story (same fetch, same slice) rather than a new slice.

**Acceptance Criteria**
- [ ] Clicking an `account_id` (from the drift table, or navigated to
      directly) fetches `GET /accounts/{id}/entries` and renders the
      response as an ordered table with a `running_balance` column
- [ ] Each row shows amount, counterparty, `recorded_at`, and running balance,
      matching the fields already returned by the endpoint — no client-side
      recomputation of balance
- [ ] A failed or timed-out entries fetch renders a bounded-time error
      naming the exact `GET /accounts/{id}/entries` URL as a direct
      fallback — no blank panel, no indefinite spinner (new this pass)
- [ ] Underlying data asserted by `milestone-05-entry-traceability.feature`
      (referenced by `docs/feature/ledger-core/feature-delta.md` line 164);
      the SPA's table rendering, click-through interaction, and
      fetch-failure handling are the
      documented untested seam per DDR-2

### US-4 — Console failure does not strand the operator
`job_id: J4` | slice 04

As the platform operator, when the console's own verdict fetch fails, I am
told immediately and shown where else to get the same answer.

**Elevator Pitch**
Before: if the console's verdict fetch fails, I see a blank page or an endless spinner and have no idea whether the books balance.
After: open `http://localhost:8080/console` when `GET /console/verdict` is unreachable → sees an explicit error message naming `GET /health/trial-balance` as the fallback path to the same answer.
Decision enabled: whether to keep waiting on the console or switch immediately to the API fallback.

**UAT Scenarios (BDD)**

```gherkin
Scenario: A failed verdict fetch shows an explicit error, not a blank page
  Given the console page has loaded but GET /console/verdict returns an error or times out
  When the operator waits for the page to respond
  Then the console shows an explicit error message instead of a blank page or an unending spinner

Scenario: The error names the fallback path
  Given the console's verdict fetch has failed
  When the operator reads the error message
  Then it names GET /health/trial-balance as a working alternative path to the verdict

Scenario: The console recovers once the API is reachable again
  Given the console previously showed a fetch-failure error
  When the underlying API becomes reachable again and the operator reloads the console
  Then the console shows the verdict normally, as in US-1
```

**Acceptance Criteria**
- [ ] A failed or timed-out `GET /console/verdict` request renders a visible
      error state within a bounded time (no indefinite spinner)
- [ ] The error state's text names `GET /health/trial-balance` explicitly, not
      a generic "try again later"
- [ ] Reloading after the API recovers returns the console to normal US-1
      behavior with no stale error state left showing
- [ ] This story has no equivalent HTTP-contract scenario in
      `milestone-04-proof-of-balance.feature` — it is pure SPA fetch-failure
      handling and stays inside DDR-2's documented untested seam; verified by
      manual dogfood demo only (see § Definition of Done)

**Slice composition gate**: every slice contains exactly one story, and
every story is user-visible (none is `@infrastructure`). PASS. (US-1 and
US-3 each gained one scenario in the comprehensive Phase 2 redo — 6 and 5
respectively — still within the 3–7 DoR range; see § DoR Validation.)

---

## Wave: DISCUSS / [REF] Acceptance Criteria

Embedded per story above. Cross-cutting AC applying to every slice:

- [ ] Console requires the same seeded operator API key as every other
      surface; unauthenticated access is refused the same way the HTTP API
      refuses it
- [ ] No slice modifies backend code — the SPA is a pure client of endpoints
      that already exist and are already contract-tested
- [ ] Every slice ships with a manual dogfood demo against a freshly built
      stack (mirrors the parent feature's `make demo-NN` convention, adapted
      for a browser step that cannot run headless in the existing pipeline)

---

## Wave: DISCUSS / [REF] Definition of Done

Authored for this feature, adapting the parent feature's list to a
frontend-only, contract-reuse scope.

1. All story AC checked and passing
2. Console renders the verdict sentence before any figures, for healthy,
   empty, and corrupted ledger states (manually verified against a real
   running stack, per DDR-2 — no new browser test suite introduced)
3. Drift table and entry trace both render from existing endpoint responses
   with no client-side balance recomputation
4. Fetch-failure state (US-4) demoed by deliberately stopping the API
   mid-session and confirming the fallback message appears
5. Peer review passed (haiku reviewer per `standard` rigor)
6. Refactor pass completed (L1–L4)
7. Zero backend changes introduced
8. All four slices shipped and demoed within the release
9. Learning hypothesis explicitly confirmed or disproved in writing per slice
   (see `docs/feature/ledger-core-console/slices/`)

---

## Wave: DISCUSS / [REF] Out-of-scope

New browser-level `.feature` tests or Playwright/E2E automation (DDR-2 stays
closed this pass) · repair or correction tooling · alerting or scheduled
checks · customer-facing views · any backend endpoint change · framework
selection (React/Vue/Svelte — ADR-006 defers to DELIVER) · mobile/responsive
design · internationalization · historical trend charts or export ·
account/entry search or filtering (evaluated and explicitly **deferred**,
not silently omitted — see § Search / Filter Candidate — Evaluated and
Deferred).

**Authentication/authorization (login, sessions, roles) is explicitly
out of scope for this feature (D10).** The console continues to assume the
existing static-operator-API-key model — no login UI, no session
management, no multi-operator support — this pass; a companion feature,
tentatively `operator-authentication`, will need to land before or
alongside this one if real login is required. Until then, every Elevator
Pitch demo in § User stories assumes the operator already has a valid API
key available to paste/configure. See § Pre-requisites.

---

## Wave: DISCUSS / [REF] WS strategy

No new WS decision required. This feature inherits `ledger-core`'s
**Strategy C — real local resources** for the data it renders: the JSON
contract it consumes is already tested against a real Postgres store, never
a fake. The console SPA's own rendering is explicitly *not* exercised by an
automated WS strategy — it stays the documented untested seam per DDR-2 /
`atdd-infrastructure-policy.md` § Known gap, verified by manual dogfood demo
instead.

---

## Wave: DISCUSS / [REF] Driving ports

| Port | Surface | Slice | Status |
|---|---|---|---|
| `GET /console` | Browser (page shell, this feature) | 01 | CREATE NEW |
| `GET /console/verdict` | HTTP/JSON, consumed by the SPA | 01, 02 | Already built (`ledger-core` slice 04) |
| `GET /accounts/{id}/entries` | HTTP/JSON, consumed by the SPA | 03 | Already built (`ledger-core` slice 05) |
| `GET /health/trial-balance` | HTTP/JSON, referenced as fallback | 04 | Already built (`ledger-core` slice 04) |

All backend endpoints require the seeded operator API key; the console reuses
it (mechanism TBD in DESIGN — see Pre-requisites).

---

## Wave: DISCUSS / [REF] Pre-requisites

- `ledger-core` backend (slices 01–05) delivered and running — verdict,
  drift, and entry-trace endpoints all demoed and contract-tested
- `docs/product/` SSOT already exists — no bootstrap needed this pass
- **`operator-authentication` (tentative name), a companion feature, not
  yet chartered (D10).** This feature's 4 stories assume the operator
  already holds a valid API key by the time any Elevator Pitch "After" step
  runs — none of them build a login UI or session flow. If real login is
  required before this console ships to anyone beyond the sole dogfooding
  operator, `operator-authentication` must land before or alongside this
  feature, not after it.
- DESIGN must settle: SPA framework choice (React/Vue/Svelte, per ADR-006) ·
  how the operator API key reaches the browser under the *current*
  static-key model (env-baked build vs. dev-proxy-injected header — login
  prompt is explicitly out of scope per D10, not a DESIGN option this pass)
  · dev-time CORS/proxy setup between the SPA dev server and the Go API
  (ADR-006 § Consequences already flags this as a real concern)
- DEVOPS receives outcome KPIs only

---

## Wave: DISCUSS / [REF] Outcome KPIs

| # | Who | Does What | By How Much | Baseline | Measured By | Type |
|---|---|---|---|---|---|---|
| 1 | P2, platform operator | Determines books-balance status from the console alone, without a terminal | 100% of routine checks (0 curl calls needed) | 0% today — console doesn't exist; operator must curl `/console/verdict` or `/health/trial-balance` by hand | Manual dogfood observation during demo; self-report during first week of use | Leading |
| 2 | P2, platform operator | Identifies which account drifted and by how much using only the console UI | 100% of corruption incidents, 0 manual curl calls to `/accounts/{id}/entries` | 0% — journey mental model notes operator currently "querying the database by hand and reconstructing by eye" | Manual dogfood observation during corruption demo | Leading |
| 3 (guardrail) | — | Console verdict agrees with `GET /health/trial-balance` verdict | 100% agreement, every demo run | Already 100% at the HTTP contract level (`milestone-04-proof-of-balance.feature`); this KPI guards that the SPA never adds client-side logic that could diverge from it | Manual comparison during each demo; no client-side reconciliation logic to audit | Guardrail |

### Metric Hierarchy
- **North Star**: KPI 1 — operator resolves "does this add up?" from the
  browser alone.
- **Leading Indicators**: KPI 2 (drift explanation without a terminal).
- **Guardrail Metrics**: KPI 3 — console must never disagree with the API it
  is a thin client over.

### Hypothesis
We believe that a browser console rendering the existing verdict, drift, and
trace contract will let the platform operator (P2) resolve "does this add
up?" entirely from `http://localhost:8080/console`, closing the gap the
parent feature's demo evidence left open ("US-4's literal pitch says 'open
`/console`'... but... out of scope this pass").
We will know this is true when P2 completes a routine balance check and a
corruption investigation using only the console, with zero curl commands.

---

## Wave: DISCUSS / [REF] DoR Validation

Against the 9-item hard gate (`nw-product-owner` DoR checklist).

| # | Item | Verdict | Evidence |
|---|---|---|---|
| 1 | Problem statement clear, domain language | **PASS** | Each story's "Before" line states the operator's actual pain in domain language (curl by hand, blank page on failure) |
| 2 | User/persona with specific characteristics | **PASS** | P2 is a real named project owner with real accountability, single seeded account, documented mental model and vocabulary (`docs/product/personas/platform-operator.yaml`) |
| 3 | 3+ domain examples with real data | **PASS** | `alice-demo`, `bob`, `treasury-demo`, and the recorded `5.00`/`7.00` deltas reuse the actual Post-Merge Integration Gate demo data from `ledger-core` — not synthetic, unlike the parent feature's waived item 3 |
| 4 | UAT in Given/When/Then (3–7 scenarios) | **PASS** | US-1: 6, US-2: 4, US-3: 5, US-4: 3 — all within range (US-1 and US-3 grew by one scenario each under the comprehensive Phase 2 redo, still ≤7) |
| 5 | AC derived from UAT | **PASS** | Every story's AC checklist traces directly to its scenarios, including the two new scenarios added this pass (US-1 loading/freshness, US-3 entries-fetch failure) |
| 6 | Right-sized (1–3 days, 3–7 scenarios) | **PASS** | 4 slices, each ≤1 day (see slice briefs), 3–6 scenarios each |
| 7 | Technical notes: constraints/dependencies | **PASS** | § System Constraints, § Pre-requisites, § WS strategy all document DDR-2, ADR-006, and the now-explicit `operator-authentication` companion-feature dependency (D10) |
| 8 | Dependencies resolved or tracked | **PASS** | Single dependency (ledger-core backend) is already delivered and demoed, not merely tracked; intra-feature slice dependencies stated in § Story Map Priority Rationale |
| 9 | Outcome KPIs defined with measurable targets | **PASS** | 2 leading KPIs + 1 guardrail, each with numeric target, baseline, and measurement method |

**Result: 9 PASS / 9 — DoR fully passes. No waiver required.**

This feature clears DoR items 2 and 3 that the parent `ledger-core` feature
could only pass with a waiver, because real dogfood data now exists from the
parent feature's own delivery.

---

## Wave: DISCUSS / [REF] Wave Decisions Summary

### Key Decisions
- [D1] Journey reused and extended: `verify-the-books.yaml` S1–S4 still map
  directly onto this feature's backbone; console-specific states added in a
  companion journey doc, not a competing SSOT journey (see: § Story Map,
  `discuss/journey-console.yaml`)
- [D2] Feature-0 walking skeleton evaluated (not skipped by default):
  slice 01/US-1 serves as both story-map WS and Feature-0 WS; no separate
  slice added (see: § Walking Skeleton (Feature-0) Evaluation)
- [D3] DDR-2 stays closed: no new browser-level `.feature` tests proposed;
  SPA rendering remains a documented untested seam (see: § System Constraints)
- [D4] Real dogfood data used throughout, closing the parent feature's DoR
  gaps on items 2 and 3 (see: § DoR Validation)
- [D9] Search/filter evaluated against Elephant Carpaccio taste tests and
  explicitly deferred, not added (see: § Search / Filter Candidate)
- [D10] Auth/authz confirmed out of scope, with a named companion feature
  (`operator-authentication`) and explicit demo assumptions (see: §
  Out-of-scope, § Pre-requisites)

### Requirements Summary
- Primary jobs/user needs: P2 needs to answer "does this add up?" and, when
  the answer is no, explain why — from a browser, without a terminal
- Walking skeleton scope: US-1 (console states the verdict), doubling as
  this feature's Feature-0 WS — see § Story Map and § Walking Skeleton
  (Feature-0) Evaluation
- Feature type: user-facing

### Constraints Established
- No backend code changes permitted in this feature
- No new automated browser testing introduced (DDR-2)
- Framework choice deferred to DELIVER (ADR-006)
- `${fetched_at}` is client-sourced only, never a server field (see
  `discuss/shared-artifacts-registry.md`)
- Auth/authz is out of scope; a companion feature is required before real
  login is possible (D10)

### Upstream Changes
- None from DISCOVER. This feature does not change any DISCOVER or
  prior-DISCUSS assumption — it fulfills a scope item (`web/console/`) that
  `ledger-core` explicitly deferred, on the SSOT and contract terms that
  scope item already fixed.
- Within-DISCUSS correction: see § Amendment Log below for what changed
  versus this feature's own prior DISCUSS pass.

---

## Wave: DISCUSS / [HOW] Amendment Log (2026-08-25)

The prior DISCUSS pass over `ledger-core-console` used decision-point
answers the orchestrator picked unilaterally rather than routing to the
user (Decision 2 defaulted to "No", Decision 3 defaulted to "Lightweight").
The user's real answers corrected both. This log records the diff for
traceability; the rest of this document has already been updated in place
— nothing here duplicates content, it summarizes where to find it.

| # | What changed | Where |
|---|---|---|
| 1 | Walking Skeleton evaluated explicitly instead of defaulted to "No"; concluded slice 01/US-1 already serves as Feature-0 WS | § Locked decisions (D2), § Walking Skeleton (Feature-0) Evaluation |
| 2 | Phase 2 Journey Design rerun as a genuine comprehensive pass specific to the console (not a formalization of `verify-the-books.yaml`) | § Locked decisions (D3), `discuss/journey-console-visual.md`, `discuss/journey-console.yaml`, `discuss/shared-artifacts-registry.md` |
| 3 | Two new console-specific states surfaced by the comprehensive pass: loading/freshness (US-1), and entries-fetch failure at the peak-tension moment (US-3) | § User stories (US-1, US-3 UAT + AC), `discuss/journey-console-visual.md` |
| 4 | New shared artifact discovered and tracked: `${fetched_at}` (client-sourced, not server) | `discuss/shared-artifacts-registry.md` |
| 5 | Search/filter candidate evaluated against Elephant Carpaccio taste tests and explicitly deferred | § Search / Filter Candidate — Evaluated and Deferred (D9) |
| 6 | Auth/authz out-of-scope decision sharpened: named companion feature (`operator-authentication`), explicit demo assumption | § Out-of-scope, § Pre-requisites (D10) |
| 7 | DoR scenario counts updated (US-1: 5→6, US-3: 4→5); DoR result unchanged (9/9 PASS) | § DoR Validation |
| 8 | `verify-the-books.yaml` SSOT: new changelog entry pointing to the companion journey docs; no S1–S4 field altered | `docs/product/journeys/verify-the-books.yaml` |

**Not changed**: DDR-2 constraint (D3-original numbering, now folded into §
System Constraints) · Elevator Pitch mandate · the 4 core stories (US-1
through US-4) and their `job_id` traceability to J4/J5 · JTBD bridge (D4,
unchanged) · framework-neutrality (D8) · overall scope verdict (PASS, no
split).

---

## Wave: DESIGN / [REF] System-Level Scope Confirmation

*Owner: nw-system-designer · interaction mode: Guide me*

The user's direct answer, asked before this section was written: the console
SPA introduces no new system-level architecture concern — no scaling,
caching, distributed state, or message-queue need; it is a single client of
the existing single-service backend. Verified below rather than
rubber-stamped, against three concrete risks named in the DESIGN task brief.

| Risk checked | Finding | Changes system architecture? |
|---|---|---|
| Does serving a static SPA bundle change the deployment shape? | No. `docs/product/architecture/brief.md` § System Architecture already states "Single Go binary plus a static asset bundle for the SPA, plus one PostgreSQL instance" — written during `ledger-core`'s own DESIGN wave, before this feature existed, specifically anticipating this bundle. `ledger-core-console` adds zero new deployables, hosts, or database roles | No |
| Does ADR-006's "separate SPA" decision imply a second deployment topology (SPA hosted independently of the Go binary)? | No. "Separate" in `adr-006-separate-spa-console.md` is a toolchain/rendering decision — client-rendered TypeScript app vs. Go-templated HTML — not a deployment-topology one. Production still serves the built bundle from the existing single-service shape; neither the ADR's Decision nor its Consequences section proposes a second production host, CDN, or reverse proxy | No |
| Does the SPA dev server need anything system-level (reverse proxy, gateway) in local dev? | Real, and already flagged by ADR-006 § Consequences: "CORS or a dev proxy is now a real concern in local development." But it is **build tooling** — a dev-server proxy entry or CORS headers scoped to the `clean` environment — not a new infrastructure component: no new server process, no load balancer, no persistent state, no rung on the scaling ladder. Recorded as an open item for DELIVER below, not designed here | No |

**Estimation** (numbers before intuition, even when the answer is "no
change" — per `nw-sd-framework`): P2 is a single seeded operator; § Persona
ID confirms no second user of the console. Peak load: low single-digit
requests/minute during a dogfood session. This does not reach the scaling
ladder's first rung (a load balancer, justified only once a single server
saturates) — nowhere close. No QPS/storage/bandwidth table is produced
because there is no scale axis to estimate against; the number that matters
here is "1 concurrent operator," not throughput.

**Conclusion: the user's answer is confirmed, not overridden.** No scaling,
caching, distributed-state, or message-queue concern is introduced by this
feature. No ADR is written at the system level for `ledger-core-console` —
there is no infrastructure decision to record, only a confirmation that none
was needed (Core Principle 3: never introduce a component without a
bottleneck to justify it — the inverse holds too, absence of a bottleneck is
itself the finding).

---

## Wave: DESIGN / [REF] Decisions

| ID | Decision | Verdict |
|---|---|---|
| SD-D1 | Console SPA adds no new system-level architecture concern | CONFIRMED — see § System-Level Scope Confirmation |
| SD-D2 | Deployment shape unchanged from `ledger-core`'s DESIGN: one Go binary + static bundle + one PostgreSQL instance | CONFIRMED — no new deployable |
| SD-D3 | No new infrastructure component (cache, queue, load balancer, CDN, shard) justified by this feature | CONFIRMED — no bottleneck exists to justify one |

---

## Wave: DESIGN / [REF] Open questions

None of these are system-level infrastructure decisions; none block the
Full-stack DESIGN handoff to `nw-ddd-architect` and `nw-solution-architect`:

- **Dev-time CORS/proxy mechanism** (ADR-006 § Consequences) — dev-proxy
  config vs. CORS headers on the `clean` environment, for the SPA-dev-server
  → Go-API cross-origin call. Build-tooling choice, decided alongside
  framework selection. Owner: DELIVER, informed by DEVOPS's `clean`
  environment definition
- **Operator API key delivery mechanism** (env-baked build vs.
  dev-proxy-injected header, per § Pre-requisites) — an application-level
  config decision, not an infrastructure one; no new component either way.
  Owner: `nw-solution-architect` or DELIVER
- **SPA framework selection** (ADR-006, D8) — explicitly deferred to
  DELIVER; framework-neutral throughout DESIGN

---

## Wave: DESIGN / [REF] Reuse Analysis

| Existing Component | File | Overlap | Decision | Justification |
|---|---|---|---|---|
| Deployment shape (single Go binary + static bundle + PostgreSQL) | `docs/product/architecture/brief.md` § System Architecture | Already anticipates serving the console's static bundle | EXTEND (documentation cross-reference only — no code or infra change) | Written during `ledger-core`'s DESIGN specifically to include "a static asset bundle for the SPA"; this feature's SPA is that bundle. Nothing to create at the system level |

No new system-level component is created. Greenfield only at the
application level (`web/console/` itself), which is `nw-solution-architect`'s
Reuse Analysis to state in its own section.

---

## Wave: DESIGN / [REF] Domain Model Scope Confirmation

*Owner: nw-ddd-architect · interaction mode: Guide me*

The user's direct answer, asked before this section was written: the console
SPA introduces no new domain model or bounded-context concern — no new
aggregate, invariant, or bounded context; it is a pure client of existing
domain concepts. Verified below against three concrete checks, rather than
rubber-stamped.

| Check | Finding | New domain concept / invariant? |
|---|---|---|
| Does anything the SPA newly displays deserve modelling, even client-side? | Two candidates scrutinized. `${fetched_at}` (`discuss/shared-artifacts-registry.md`) is a browser-clock timestamp labelling freshness — no invariant depends on it, it is never persisted or compared by the core, and the registry itself flags it must be re-pointed (not dual-sourced) if a server timestamp ever appears. The drift table row (`account_id`, `stored`, `computed`, `delta`) is a field-for-field rendering of the existing Drift concept and the `drifted` array already returned by `GET /console/verdict` — not a new shape. Both stay presentation state | No |
| Does the Ubiquitous Language table need a new term for console vocabulary? | "Verdict" was checked against the existing table (`docs/product/architecture/brief.md` § Domain Model). It was **missing**, but it is not console-invented: it is already load-bearing in `ledger-core`'s own domain modelling (DDD-21, "the verdict withholds; it never accuses", `docs/feature/ledger-core/feature-delta.md`) and its HTTP contract (`GET /console/verdict`). This is a glossary backfill closing a pre-existing gap, not a new domain-modelling decision this feature introduces | Backfill only — added to the existing table |
| Is any new invariant introduced? | No. US-3's AC is explicit — "no client-side recomputation of balance" — and every UAT scenario across US-1–US-4 renders fields verbatim from the two already-tested endpoints. I3 (`docs/product/architecture/brief.md` § Invariants) remains the sole deliberately-unenforced invariant, unchanged by this feature; there is nothing for a client-side domain core to guard, because there is no client-side domain core | No |

**Conclusion: the user's answer is confirmed, not overridden.** No bounded
context, aggregate, or invariant is added by `ledger-core-console`. One
glossary backfill ("Verdict") is recorded, sourced from a pre-existing
backend decision (DDD-21), not a new one made here. Full detail, including
the extended Ubiquitous Language table and the presentation-state
determination for both scrutinized candidates:
`docs/product/architecture/brief.md` § Domain Model → "Console SPA
(`ledger-core-console`, confirmed 2026-08-25)".

---

## Wave: DESIGN / [REF] Decisions

| ID | Decision | Verdict |
|---|---|---|
| DDD-D1 | Console SPA adds no new bounded context, aggregate, or invariant | CONFIRMED — see § Domain Model Scope Confirmation |
| DDD-D2 | `${fetched_at}` and the drift table row are presentation state, not domain concepts | CONFIRMED — neither carries an invariant or is persisted/compared by the domain core |
| DDD-D3 | "Verdict" added to the Ubiquitous Language glossary, backfilling a gap from `ledger-core`'s own DDD-21, not introducing a new domain concept (the term is new to this table; the concept it names is not new to the domain) | CONFIRMED — see `docs/product/architecture/brief.md` § Domain Model → Ubiquitous language |

---

## Wave: DESIGN / [REF] Reuse Analysis

| Existing Component | File | Overlap | Decision | Justification |
|---|---|---|---|---|
| Ledger bounded context, Account/Entry/Transaction aggregates, Ubiquitous Language table | `docs/product/architecture/brief.md` § Domain Model | Console renders Account, Entry, Trial balance, Drift, and (now-catalogued) Verdict exactly as already modelled | EXTEND (glossary backfill only — "Verdict" row added; no aggregate or invariant touched) | The console is a pure downstream consumer; no new bounded context or tactical pattern is warranted for a read-only rendering surface over an already-tested contract |

No new bounded context or aggregate is created. Handoff to
`nw-solution-architect` for application-level design (component
decomposition, API-key delivery mechanism, dev-proxy/CORS) is unblocked —
nothing here constrains its choices beyond what `docs/product/architecture/brief.md`
§ Domain Model already states.

---

## Wave: DESIGN / [REF] Application Architecture

*Owner: nw-solution-architect · interaction mode: Guide me · third and final
architect in this feature's Full-stack DESIGN sequence*

The three open questions § Pre-requisites named for this wave are now
settled — the user answered all three directly, in Guide mode, before this
section was written (React; client-side paste/localStorage key; Vite
dev-proxy). Full design lives in `docs/product/architecture/brief.md` §
Application Architecture → "Console SPA (`ledger-core-console`, confirmed
2026-08-25)"; this section is the pointer plus the decision table DISTILL
and DEVOPS read from `feature-delta.md` directly.

**SPA framework and toolchain**: React 18, built with Vite 5 +
`@vitejs/plugin-react`. CRA was checked and correctly not defaulted to — it
is deprecated. Next.js and Parcel were considered and rejected (Next's
server/routing machinery is unused complexity for a single client-rendered
page; Parcel has a thinner React-proxy-config ecosystem for the exact
dev-server-to-Go-API need this feature has). Full rationale:
`docs/product/architecture/brief.md` § Technology choices.

**Key delivery mechanism**: `ApiKeyPrompt` component gates first load when
no key is stored; `keyStorage` (the sole `localStorage` accessor) persists
it under `ledgerops_console_api_key`; `apiClient` (the sole `fetch()`
caller) attaches `Authorization: Bearer <key>` — the exact header/scheme
read from `internal/adapters/http/router.go:60-73`, not assumed — to every
request. A `401 unidentified_caller` response clears the stored key and
re-prompts with an inline "key rejected" message rather than retrying
silently. Full flow, including the rejected env-baked and proxy-injected
alternatives: `adr-010-console-client-side-api-key.md`.

**Fetch timeout (concrete value)**: slice-03 and slice-04 both require a
"bounded-time" error state but explicitly deferred the concrete number to
DESIGN. Settled: `apiClient` applies a single 8-second `AbortController`
timeout to every request, treated identically to a network failure (no
separate "timed out" error class the operator has to distinguish). 8s
rationale: single local operator, co-located Go binary, expected
millisecond round-trips per § System-Level Scope Confirmation — generous
enough to absorb a cold start, still reads as "immediate" against US-4's
pitch. Fixed constant, not configurable this release. Full detail:
`docs/product/architecture/brief.md` § Console SPA → "Fetch timeout".

**Dev-time proxy**: Vite dev server proxies exactly three path prefixes
(`/console/verdict`, `/accounts`, `/health`) to the local Go binary — no
catch-all. Confirmed local-dev-tooling-only: there is no hosted environment
(`clean`/`ci` are the entire matrix, per `docs/product/architecture/brief.md`
§ Deployment shape), so "production" here is the built static bundle served
by the same Go binary at the same origin, where neither CORS nor the proxy
applies. No CORS headers were added to the Go API — that was the option
DDR-2 exists to rule out.

**Component decomposition** (`web/console/`): `ConsoleApp` (orchestrator),
`ApiKeyPrompt` (US pre-requisite), `VerdictBanner` (US-1, with loading/error/
freshness states), `DriftTable` (US-2), `EntryTrace` (US-3, three states —
loading/loaded/error, mirroring `VerdictBanner`'s S1/S1a pattern per
`discuss/journey-console-visual.md` S3a/S4a), `VerdictFetchError` (US-4),
`apiClient` (driven port — sole `fetch()` caller, GET-only, no write methods
exposed per Core Principle 12's read/write port-splitting rule, single 8s
timeout on every request), `keyStorage` (sole `localStorage` accessor). Full
table with contract-shape classification per component:
`docs/product/architecture/brief.md` § Console SPA → "Component
decomposition".

**C4 diagrams**: System Context and Container (updated: concrete React
label + auth-flow annotation, zero topology change) inherited from §
System Architecture; a new Component-level (L3) diagram for `web/console/`
internals is added, since the SPA now has 8 named components — see
`docs/product/architecture/brief.md` § Console SPA.

**Enforceable rule**: `apiClient`/`keyStorage` as sole `fetch`/`localStorage`
accessors, recommended enforcement via `eslint-plugin-boundaries` or a
directory-scoped `no-restricted-imports` rule in the CI lint job — the
console-side equivalent of the Go side's `exhaustive`-linter obligation
(DDD-12/DDD-17).

**External integrations**: none. The console's only integration is the
same-org Go API, already contract-tested. No contract-testing annotation
added to the DEVOPS handoff.

**Earned Trust**: DDR-2 forbids a new automated browser-level probe for
`apiClient`'s network dependency. The manual dogfood demo US-4's UAT and
this feature's Definition of Done item 4 already require (stop the API
mid-session, confirm the named fallback appears) *is* that probe, executed
per-release rather than in CI — an explicit trade-off given DDR-2, not a
silent gap. `localStorage`-unavailable is flagged as an accepted, unbuilt
gap (see brief.md for the fallback design if it ever becomes a real dogfood
failure).

---

## Wave: DESIGN / [REF] Decisions

| ID | Decision | Verdict |
|---|---|---|
| SA-D1 | SPA framework: React 18, toolchain: Vite 5 | CONFIRMED — user's direct answer; CRA deprecated, Next.js/Parcel rejected with rationale |
| SA-D2 | Operator API key: client-side paste, `localStorage`, `Authorization: Bearer` header | CONFIRMED — see `adr-010-console-client-side-api-key.md` |
| SA-D3 | Dev-time CORS/proxy: Vite dev-server proxy over exactly 3 path prefixes, local-dev-only | CONFIRMED — no CORS headers added to the Go API; no hosted environment exists to make "production" mean anything but the same-origin built bundle |
| SA-D4 | 8-component decomposition for `web/console/`, mapped US-1–US-4 | CONFIRMED — see brief.md § Console SPA → Component decomposition |
| SA-D5 | No new bounded context/aggregate/system component; console is a pure client | CONFIRMED — inherited from SD-D1–D3 and DDD-D1–D3, unchanged by application-level design |
| SA-D6 | Fetch timeout: 8s, single constant applied to every `apiClient` request, treated as a network failure on expiry | CONFIRMED — closes the concrete-timeout obligation slice-03/slice-04 AC left open for DESIGN |

---

## Wave: DESIGN / [REF] Reuse Analysis

| Existing Component | File | Overlap | Decision | Justification |
|---|---|---|---|---|
| *(none — `web/`, `web/console/` verified absent via `Glob web/**`)* | — | — | CREATE NEW | Genuinely greenfield at DESIGN time; no prior frontend code, toolchain, or dev-server config anywhere in the repo |
| `requireOperatorKey` auth middleware | `internal/adapters/http/router.go:60-73` | Defines the exact contract (`Authorization: Bearer`, `401 unidentified_caller`) the console's key-delivery mechanism is designed against | EXTEND (contract reuse only, zero backend code touched) | Read directly from source, not assumed — the header scheme and refusal shape driving `ApiKeyPrompt`/`apiClient` are exact |
| `GET /console/verdict`, `GET /accounts/{id}/entries`, `GET /health/trial-balance` wire shapes | `docs/product/outcomes/registry.yaml` OUT-4, OUT-5 | Console renders these shapes verbatim, no new field, no client-side derivation | EXTEND (consumption only) | Matches "no client-side recomputation of balance" AC across US-1–US-3 |

Full application-level Reuse Analysis (identical content, SSOT copy):
`docs/product/architecture/brief.md` § Application Architecture → Reuse
Analysis → "Console SPA reuse pass".

---

## Wave: DESIGN / [REF] Outcome Collision Check

`nwave-ai outcomes check-delta` could not be run directly — this agent
invocation has no `Bash` tool available, and a prior session already
recorded `nwave-ai outcomes register` failing with `FileNotFoundError` on
its own packaged `schema.json` (memory: `nwave-outcomes-register-cli-broken`,
2026-08-18). Rather than silently skip, `docs/product/outcomes/registry.yaml`
(10 rows, OUT-1–OUT-10, all `feature: ledger-core`) was read and manually
cross-checked against this feature's design instead.

**Finding: no collision, and no new candidate outcome to register.**
`ledger-core-console` introduces no new HTTP endpoint, no new
operation/specification/invariant — it is a pure client of OUT-4 (`GET
/accounts/{id}/entries`) and OUT-5 (`GET /console/verdict` /
`GET /health/trial-balance`), rendering their existing shapes verbatim. Per
the collision-check's own gate-scoping (D-6, "code-feature pipelines only")
and its skip condition for features with "no new typed contract surface",
this feature's DESIGN output does not add a row to the registry. Owner
(DEVOPS or a later session with `Bash` access) should still attempt the CLI
directly once the packaging bug is fixed, to confirm this manual finding
rather than let it stand unverified indefinitely.

---

## Wave: DESIGN / [REF] Open questions — resolved

All three items § Pre-requisites flagged for this wave are now closed:

| Item | Resolution |
|---|---|
| SPA framework selection | React 18 (SA-D1) |
| Operator API key delivery mechanism | Client-side paste/`localStorage` (SA-D2, ADR-010) |
| Dev-time CORS/proxy setup | Vite dev-server proxy, local-dev-only (SA-D3) |

No open question remains blocking DEVOPS or DELIVER for this feature.

---

## Wave: DESIGN / [REF] Wave Decisions Summary

### Key Decisions
- [SA-D1] React 18 + Vite 5 toolchain — CRA deprecated, Next.js/Parcel
  rejected (see: `docs/product/architecture/brief.md` § Technology choices)
- [SA-D2] Client-side paste/`localStorage` API key delivery, `Authorization:
  Bearer` header, 401-triggers-reprompt flow (see:
  `adr-010-console-client-side-api-key.md`)
- [SA-D3] Vite dev-server proxy (3 exact path prefixes), confirmed
  local-dev-tooling-only — no hosted environment exists, no CORS headers
  added to the Go API (see: `docs/product/architecture/brief.md` § Console
  SPA → Dev-time proxy)
- [SA-D4] 8-component decomposition (`ConsoleApp`, `ApiKeyPrompt`,
  `VerdictBanner`, `DriftTable`, `EntryTrace`, `VerdictFetchError`,
  `apiClient`, `keyStorage`), each mapped to US-1–US-4 and classified by
  contract shape (see: `docs/product/architecture/brief.md` § Console SPA →
  Component decomposition)
- [SA-D6] 8-second fetch timeout, single constant, closes the concrete-value
  obligation slice-03/slice-04 AC deferred to DESIGN (see:
  `docs/product/architecture/brief.md` § Console SPA → "Fetch timeout")

### Architecture Summary
- Pattern: same modular monolith / ports-and-adapters as `ledger-core`; the
  console is a pure driving-side client, no new backend port
- Paradigm: console-internal render layer is functional (pure-function
  components); `apiClient`/`keyStorage` are the only two effectful modules,
  isolated by design (Core Principle 12)
- Key components: see § Reuse Analysis and § Application Architecture above

### Reuse Analysis
See § Reuse Analysis above (identical content mirrored into
`docs/product/architecture/brief.md`).

### Technology Stack
- React 18: user's direct DESIGN answer; MIT-licensed, largest OSS
  ecosystem among the multi-paradigm-TS options
- Vite 5 + `@vitejs/plugin-react`: MIT-licensed, 2026 default for new React
  SPAs, not CRA (deprecated), not Next.js (unused server machinery), not
  Parcel (thinner proxy-config ecosystem for this feature's exact need)

### Constraints Established
- Zero backend code changes (DDR-2, held — no CORS headers added to the Go
  API, no auth middleware change)
- `apiClient`/`keyStorage` are the sole effectful modules; every other
  component is a pure render function (enforced by ESLint boundary rule,
  recommended to DEVOPS)
- No contract-testing annotation needed — no external/third-party
  integration introduced

### Upstream Changes
- None. This wave confirms and builds on SD-D1–D3 (system, no change) and
  DDD-D1–D3 (domain, no change); no DISCUSS assumption is altered.

**Handoff to `nw-platform-architect` (DEVOPS)**: ready. No external
integrations requiring contract tests. Open items for DEVOPS: recommend
wiring the `eslint-plugin-boundaries` (or equivalent) rule into the existing
CI lint job; confirm the Vite build output path (`web/console/dist/`, or
equivalent) is what the Go binary's static-asset embedding step serves at
`/console` in the `clean`/`ci` environments.

---

## Wave: DEVOPS / [REF] Infrastructure inheritance

*Owner: nw-platform-architect (Apex) · interaction mode: user's explicit
choice — inherit, not re-decide*

The user chose not to re-run the full 9-decision DEVOPS walkthrough for this
feature. Every infrastructure decision below is inherited from `ledger-core`'s
own DEVOPS wave (`docs/feature/ledger-core/feature-delta.md` § Wave: DEVOPS,
and `docs/product/architecture/brief.md` § Deployment shape), extended only
where this feature's TypeScript SPA genuinely adds surface area:

| Decision | Inherited value | Extended for this feature? |
|---|---|---|
| Deployment target | None — no hosted environment exists (`brief.md` § Deployment shape) | No — this feature does not change that |
| Container orchestration | Docker Compose (`docker-compose.yml`) | No new service added |
| CI/CD platform | GitHub Actions (`.github/workflows/ci.yml`, `nightly.yml`) | Yes — one new job, `console`, added to `ci.yml` (see § CI/CD pipeline outline) |
| Existing infrastructure | Yes, both infra and CI/CD exist (brownfield extension) | Confirmed — `ci.yml` read in full before extending, `web/` verified absent (DESIGN § Reuse Analysis) |
| Observability/logging | None — all outcome KPIs are CI assertions or manual, no Prometheus/Datadog/ELK | No new stack; this feature's 3 KPIs are manual/self-report only (§ Monitoring contracts below) |
| Deployment strategy | Recreate | Unchanged — no deployable exists to strategize a rollout for |
| Continuous learning | No — no monitoring/alerting infra to extend | Unchanged, not applicable to a single-operator dogfood tool |
| Git branching strategy | Trunk-based development | Unchanged — the new `console` job runs on the same `push`/`pull_request` triggers as every other job |
| Mutation testing strategy | `nightly-delta` (`CLAUDE.md` § Mutation Testing Strategy, project-wide) | Applies project-wide; see § Mutation testing strategy below for the concrete gap this creates for TS code |

---

## Wave: DEVOPS / [REF] Environment matrix

Full detail: `docs/feature/ledger-core-console/environments.yaml`.

| Environment | Purpose | CI-gated? |
|---|---|---|
| `clean` | Fresh clone, `web/console/` built from zero | Indirectly — feeds the `ci` job |
| `ci` | GitHub Actions ubuntu-latest — `npm ci`, ESLint, `tsc --noEmit`, `vite build` | Yes — new `console` job |
| `with-stored-key` | Browser already holds a valid operator API key | No — manual dogfood parametrization only (DDR-2) |
| `without-stored-key` | First load, `ApiKeyPrompt` gate | No — manual dogfood parametrization only |
| `api-unreachable` | Go API stopped/unreachable mid-session | No — manual dogfood parametrization only |

**Path convention note**: this file lives at
`docs/feature/ledger-core-console/environments.yaml`, not under a `devops/`
subdirectory — `ledger-core`'s own `environments.yaml` used
`docs/feature/ledger-core/devops/environments.yaml` (pre-lean-v3.14
precedent), but the current `nw-devops` skill's Outputs contract states the
no-subdirectory path for lean v3.14. This feature's `feature-delta.md` is
already written in that style, so the current convention was followed and the
divergence from the sibling precedent is flagged here rather than silently
introduced.

---

## Wave: DEVOPS / [REF] CI/CD pipeline outline

New job `console` added to `.github/workflows/ci.yml`, in the commit-stage
section (parallel with `lint`/`build`/`unit`/`property` — no database
dependency, matching DESIGN's confirmation that the SPA is a pure client with
no Testcontainers need):

| Step | What | Gate |
|---|---|---|
| Probe | Checks for `web/console/package.json` | Guards every later step — this feature's DELIVER has not landed yet, so the job is a `::notice`-only no-op until it does, rather than red-CI-by-absence |
| `npm ci` | Installs pinned deps from `package-lock.json` | Blocking once `web/console/` exists |
| ESLint (incl. `eslint-plugin-boundaries`) | Enforces `apiClient`/`keyStorage` as sole `fetch()`/`localStorage` accessors — DESIGN's flagged item, now wired | Blocking |
| `tsc --noEmit` | Type-check without emitting build output | Blocking — added as a separate step, mirroring the Go side's `go vet` / `golangci-lint` separation, since ESLint's TS rules alone do not guarantee full type-checker coverage |
| `vite build` | Production bundle | Blocking |

Trigger rules: same `on: [push, pull_request]` as every other job — no new
trigger logic, consistent with trunk-based development (main is expected
always releasable, so the gate runs on every push, not just release
branches).

**Local quality gates**: no pre-commit/pre-push hook framework exists yet in
this repo (`environments.yaml`'s `coexistence_matrix` for `ledger-core`
already flags `pre-commit` as "not yet installed"). No new local-gate
obligation is added by this feature beyond what a developer's own `npm run
lint`/`npm run build` already provides ad hoc — introducing a hook framework
for one feature's frontend code, when the Go side has none either, would be
scope creep past what DESIGN or DISCUSS asked for.

---

## Wave: DEVOPS / [REF] Build-output wiring — resolved

DESIGN's second flagged item for DEVOPS: confirm where the Go binary serves
the static bundle. At the point this was flagged, nothing served `/console`
— `NewRouter` registered only API routes, no `http.FileServer`, no
`embed.FS`, no static-asset route of any kind, and `web/` did not exist in
the repository.

**Decision (2026-08-25, user, explicit)**: treat the static-file-serving
route as a DEVOPS/infra concern, not a fifth console user story. Rationale
accepted: a minimal static-file-serving route is infrastructure — like the
health endpoint — not console feature business logic. DDR-2's and this
feature's Definition of Done's "no backend changes" language is scoped to
US-1..US-4's *behavior* (the four SPA-rendering stories), not to the infra
wiring required to serve the SPA's own HTML shell at all. This resolves the
two options the open item left on the table in favor of option 1
(DEVOPS/infra concern, in scope for this DEVOPS wave).

**What was built** (`internal/adapters/http/`):

- `router.go` — the operator-API-key middleware moved from the whole router
  onto a `chi.Router.Group` scoped to the JSON API routes only
  (`/accounts`, `/transfers`, `/health/trial-balance`, `/console/verdict`,
  `/metrics`). `GET /console/verdict` still requires the key — nothing about
  its own auth changed. What changed is that the key is no longer implicitly
  required for paths that were never JSON endpoints to begin with.
- `console_static.go` (new) — `mountConsole` registers `GET /console` (the
  SPA shell, served via `http.ServeFile`) and `GET /console/*` (built
  assets, e.g. `/console/assets/index-XXXX.js`, via
  `http.StripPrefix` + `http.FileServer(http.Dir(...))`) against
  `web/console/dist` — Vite's `build.outDir`, confirmed still the right path
  (unchanged from the design-only recommendation, matches `brief.md` §
  Deployment shape's "a static asset bundle for the SPA"). Both routes sit
  **outside** the operator-API-key group: the HTML shell and its JS/CSS have
  to load before any key can be presented, and DESIGN's own key-delivery
  flow (`ApiKeyPrompt`) depends on the shell being reachable key-free.
- **Absence handling**: `mountConsole` probes for
  `web/console/dist/index.html` at `NewRouter` construction time (once, at
  process startup) — the same probe-guarded posture the `console` CI job
  already established for this exact "doesn't exist yet" problem
  (`.github/workflows/ci.yml`, `console` job's own `if [ -f
  web/console/package.json ]` step). If the file is absent, **no /console
  route is registered at all**; the path 404s through chi's ordinary
  unmatched-route handling, `cmd/api/` starts normally, and no existing test
  is affected. This is true today (`web/console/` does not exist — DELIVER
  for this feature has not landed) and remains true after DELIVER lands and
  the dist files are actually present.
- Tests: `console_static_test.go` pins both states with `t.TempDir` +
  `os.Chdir` (no dependency on the repository's actual working tree) —
  dist-absent → 404, dist-present → shell and asset served with no
  `Authorization` header required, and `/console/verdict` still answers 401
  without one. `go build ./...`, `go vet ./...`, and the full `internal/...`
  and `tests/...` suites pass unchanged.

**Known follow-up, not addressed by this change**: `Dockerfile`'s final
stage copies only the compiled binary, not `web/console/dist/` — the
compose-based deployment path (`brief.md` § Deployment shape) will not
actually serve the console until the image build also copies the built
bundle in. Deferred rather than edited here: `web/console/` does not exist
yet, so wiring an unconditional `COPY --from=build .../web/console/dist
./web/console/dist` now would need either a committed placeholder directory
or a build-time conditional Docker does not natively support without a
verified `docker build` run, which is outside this change's verification
surface (`go build`/`go test`, per the task that requested this). Flagged
here rather than silently left implicit, for whoever wires Docker before
this feature is genuinely deployable end-to-end.

---

## Wave: DEVOPS / [REF] Monitoring contracts

Full detail: `docs/product/kpi-contracts.yaml` (KPI-C1, KPI-C2, KPI-C3).

| KPI | Measured by | Operational requirement |
|---|---|---|
| KPI-C1 (leading) — books-balance check without a terminal | Manual dogfood observation during demo; self-report first week of use | A demo checklist item, not a dashboard: operator opens the console with a stored key (`with-stored-key` environment) and confirms zero `curl` calls were needed to answer "does this add up?" |
| KPI-C2 (leading) — drift explanation without a terminal | Manual dogfood observation during corruption demo | Same checklist, corruption-demo variant: confirm zero `curl /accounts/{id}/entries` calls were needed |
| KPI-C3 (guardrail) — console/health-check verdict agreement | Manual comparison during each demo | Structural guardrail, not a metric — holds because `GET /console/verdict` and `GET /health/trial-balance` share `verdictHandler` (`router.go`); the checklist item is a design-review trip-wire ("did this PR add client-side verdict logic?"), not a runtime check |

**Why no telemetry infrastructure is built**: DISCUSS's own Outcome KPIs
table states all three as "Manual dogfood observation" / "self-report",
explicitly not automated telemetry — consistent with the parent feature's
framing (no hosted environment exists to emit from, DEVOPS D1) and with
DDR-2 (no new automated browser probe). Building event collection, a
dashboard, or alerting for KPIs DISCUSS itself scoped as manual would be
over-engineering against Core Principle 3's own logic (no bottleneck exists
here — there is no running production surface to instrument). The demo
checklist above is the entire "instrumentation."

---

## Wave: DEVOPS / [REF] Deployment strategy

Recreate — inherited unchanged from `ledger-core` (`brief.md` § Deployment
shape). No new deployable exists for this feature to strategize a rollout
for; the console ships as part of the same single Go binary + static bundle
`ledger-core` already established, once § Build-output wiring above is
resolved. Rollback contract: identical to `ledger-core`'s — redeploy the
previous image tag. No frontend-specific rollback concern exists (no
database migration, no schema) beyond ensuring the previous bundle is still
buildable from the previous commit, which trunk-based development's
always-releasable-main discipline already guarantees.

---

## Wave: DEVOPS / [REF] Mutation testing strategy

**Selected**: `nightly-delta`, unchanged — already declared project-wide in
`CLAUDE.md` § Mutation Testing Strategy and re-confirmed here rather than
re-decided, per the user's inherit-existing-infrastructure choice.

**The concrete gap this creates for TypeScript**, flagged rather than
silently assumed away: `nightly.yml`'s existing `mutation-delta` job scopes
itself to `*.go` files only (`git log --name-only ... -- '*.go'`) and is
already honest that no mutation-testing tool is vendored for Go
(`go.mod`/`go.sum` carry none). No JavaScript/TypeScript mutation-testing
tool (e.g. Stryker Mutator) is vendored in this repo either, and none is
added by this DEVOPS wave. Once `web/console/` exists, `nightly-delta`
mutation testing does not extend to it automatically — the `mutation-delta`
job's file-glob would need a second branch (`*.ts`/`*.tsx`) and a vendored
tool before the project-wide `nightly-delta` strategy is actually true for
the TS code, not merely declared to be. Recorded as an open item for
whichever DEVOPS pass first touches `web/console/`'s CI wiring in earnest
(likely this feature's own DELIVER, or a fast-follow) — not silently
deferred past visibility.

---

## Wave: DEVOPS / [REF] Observability stack

None — inherited unchanged. No Prometheus/Datadog/ELK/OpenTelemetry is
introduced for this feature. `kpi-contracts.yaml`'s existing
`runtime_instrumentation` section (logs/metrics/traces, all scoped to the Go
service) is untouched; the console has no server-side runtime to instrument,
and its 3 outcome KPIs are manual per § Monitoring contracts above. Logs,
metrics, and traces sections of `kpi-contracts.yaml` remain `ledger-core`
Go-service-only.

---

## Wave: DEVOPS / [REF] Branching strategy

Trunk-based development — inherited unchanged (`brief.md` § Domain Model
note on DDD-12: "required on every push per trunk-based branch protection").
The new `console` CI job uses the identical `on: [push, pull_request]`
trigger as every existing job; no branch-specific pipeline logic is
introduced for this feature.

---

## Wave: DEVOPS / [REF] Coexistence matrix

Full detail: `docs/feature/ledger-core-console/environments.yaml` §
`coexistence_matrix`. Summary: the new `console` CI job and
`eslint-plugin-boundaries` dependency are confined to `web/console/` and must
not require any change to the Go `lint` job, `golangci-lint` config, or
`nightly.yml`'s `mutation-delta` job (beyond the open item § Mutation testing
strategy already names). `make demo-01`..`make demo-05` and `docker compose`
remain unaffected until § Build-output wiring is resolved and implemented.

---

## Wave: DEVOPS / [REF] Pre-requisites

- `ledger-core` backend delivered, running, contract-tested — confirmed
  (inherited, unchanged by this wave)
- `web/console/` does not yet exist — this DEVOPS wave's CI job and
  `environments.yaml` are written defensively (probe-guarded) against that
  fact, not assuming DELIVER has already landed
- **Open, blocking DELIVER's end-to-end dogfoodability, not this wave's CI
  gate**: § Build-output wiring above must be resolved (in scope for this
  feature vs. deferred) before the console can be demoed from a built
  binary rather than only `vite dev`
- **Open, tracked, non-blocking**: § Mutation testing strategy's TS-coverage
  gap

---

## Wave: DEVOPS / [REF] Wave Decisions Summary

### Key Decisions
- [D1] All 9 DEVOPS decision points inherited from `ledger-core`'s own
  DEVOPS wave rather than re-run, per the user's explicit choice (see: §
  Infrastructure inheritance)
- [D2] One new CI job (`console`) added to `.github/workflows/ci.yml`,
  probe-guarded against `web/console/` not yet existing (see: § CI/CD
  pipeline outline)
- [D3] `eslint-plugin-boundaries` wired into the new job per DESIGN's
  flagged enforceable-architecture-rule item (see: § CI/CD pipeline outline)
- [D4] Build-output wiring (static-file-serving route) is flagged, not
  decided — genuine ambiguity against DDR-2's "zero backend changes"
  constraint, left for the user or `nw-solution-architect` (see: § Build-output
  wiring)
- [D5] All 3 outcome KPIs confirmed manual/self-report per DISCUSS; no
  telemetry infrastructure built (see: § Monitoring contracts)
- [D6] `nightly-delta` mutation testing re-confirmed project-wide; TS-side
  gap (no vendored tool, no file-glob branch) flagged as an open item, not
  silently assumed covered (see: § Mutation testing strategy)

### Infrastructure Summary
- Deployment: none (no hosted environment) + Recreate strategy, both
  inherited unchanged
- CI/CD: GitHub Actions, trunk-based triggers, one new probe-guarded job
- Observability: none — 3 outcome KPIs are manual dogfood checklist items
- Mutation testing: `nightly-delta`, project-wide, TS-coverage gap flagged

### Constraints Established
- The `console` CI job must stay a no-op until `web/console/package.json`
  exists — no red CI from this wave's change alone
- No Prometheus/Datadog/ELK/OpenTelemetry introduced
- No pre-commit/pre-push hook framework introduced for this feature alone
- Static-file-serving wiring is explicitly undecided, not silently deferred

### Upstream Changes
- None to DISCUSS or DESIGN assumptions. § Build-output wiring surfaces a
  genuine open question DESIGN's own Open Questions section did not fully
  close (it named the mechanism as TBD but not the DDR-2 boundary question
  this wave found on inspecting `router.go` directly) — recorded as an open
  item for the user/architect, not written back as a changed assumption,
  since nothing here overrides what DESIGN decided.

**Per-wave peer review**: skipped. No trigger applies — deployment target,
CI/CD platform, and observability stack are all inherited/unchanged; no
security posture change; no novel deployment target. Per `nw-devops` SKILL.md
§ Peer Review Gate, default is skip absent a trigger, and none of the four
listed triggers (novel deployment target, new CI/CD framework, observability
rewrite, security posture change) apply here.

**Handoff to `nw-acceptance-designer` (DISTILL)**: ready.
`docs/feature/ledger-core-console/environments.yaml` is written and
parses the 5 target environments DISTILL should parametrize US-1's loading
state (`without-stored-key`), US-3's entries-fetch-failure scenario
(`api-unreachable`), and US-4's fallback scenario (`api-unreachable`) over.
One open item is carried forward for DISTILL's awareness rather than
resolved here: § Build-output wiring's undecided static-serving route may
affect whether a "demo the built bundle" scenario is writable yet, versus
only a `vite dev`-mode scenario.
