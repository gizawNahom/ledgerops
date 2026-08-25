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
