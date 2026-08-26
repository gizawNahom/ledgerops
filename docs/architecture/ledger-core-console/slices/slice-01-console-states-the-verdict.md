# Slice 01 — Console states the verdict

**Job**: J4 | **Story**: US-1 | **WS strategy**: inherits `ledger-core` Strategy C (real, for the JSON contract); rendering itself is manual dogfood

## Goal

Operator opens the console in a browser and immediately knows whether the
books balance, stated in words, before any figures.

## IN scope

- Console page shell at `GET /console`
- Fetch `GET /console/verdict` on page load
- Render the `verdict` field as a plain sentence ("Books balance: YES" /
  "Books balance: NO") as the first thing on the page, above any table or
  number
- Works correctly for healthy, empty, and corrupted ledger states

## OUT scope

- Drift table (slice 02)
- Entry drill-down (slice 03)
- Fetch-failure handling (slice 04)
- Any visual styling beyond making the sentence legible and first

## Learning hypothesis

Disproves "the existing `GET /console/verdict` JSON contract is sufficient to
drive a truthful browser rendering with no additional backend work" if the
SPA cannot render the verdict sentence purely from the existing response
fields (`verdict`, `imbalance_minor`, `entry_count`, `elapsed_ms`, `drifted`).
Confirms it if a single fetch + one conditional render is all that's needed.

## Acceptance criteria

- [ ] Opening the console against a healthy ledger shows "Books balance: YES"
      as the page's first sentence
- [ ] Opening the console against an empty ledger shows "Books balance: YES"
- [ ] Opening the console against a corrupted ledger (one entry altered
      out-of-band, e.g. alice-demo +5.00) shows "Books balance: NO"
- [ ] The verdict sentence never appears below a table or figure
- [ ] While the verdict fetch is in flight, the page shows a neutral
      "Checking the books..." state, never a blank page (added: comprehensive
      Phase 2 redo, 2026-08-25 — see `discuss/journey-console-visual.md`)
- [ ] Once the verdict renders, it is labeled with a client-side
      `${fetched_at}` timestamp (browser clock, not a server field) so the
      operator can judge freshness (added: comprehensive Phase 2 redo,
      2026-08-25 — see `discuss/shared-artifacts-registry.md`)
- [ ] `make demo-console-01` (or equivalent) shows the verdict rendering
      correctly against a freshly built stack, healthy and corrupted, run
      by hand — no headless browser step introduced to CI

## Data

Real, reused from `ledger-core`'s Post-Merge Integration Gate demo:
`treasury-demo` (system), `alice-demo` (wallet, funded 100.00 from treasury),
`txn_1b575c10-...`. Corruption case: alice-demo's entry altered by 5.00
out-of-band, matching the existing demo evidence exactly.

## Dependencies

None beyond the already-delivered `ledger-core` backend (`GET /console/verdict`
live and contract-tested).

## Effort

≤1 day. Reference class: one page shell, one fetch, one conditional string
render. ADR-006 already flags the second-toolchain scaffolding (build step,
dev server) as the most likely place a console slice's one-day ceiling
breaks — budget the toolchain setup, not the rendering logic, as the risk.
