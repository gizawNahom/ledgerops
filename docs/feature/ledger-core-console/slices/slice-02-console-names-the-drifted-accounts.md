# Slice 02 — Console names the drifted accounts

**Job**: J4 | **Story**: US-2 | **WS strategy**: inherits `ledger-core` Strategy C; rendering itself is manual dogfood

## Goal

When the verdict is NO, the operator sees which accounts drifted and by how
much, without leaving the console or opening a terminal.

## IN scope

- Render the `drifted` array from the already-fetched `GET /console/verdict`
  response as a table below the verdict sentence
- One row per drifted account: `account_id`, `stored`, `computed`, `delta`
- No table rendered at all when `drifted` is empty

## OUT scope

- Entry drill-down / click-through (slice 03)
- Fetch-failure handling (slice 04)
- Sorting, filtering, or pagination of the drift table (no evidence yet that
  drift lists grow large enough to need it)

## Learning hypothesis

Disproves "a single fetch can carry both the verdict and the full drift list
in one round trip, rendered as a plain table" if the `drifted` array turns
out too large or shaped wrong for a direct table render — surfacing a real
need for pagination or a second endpoint before slice 03 is built on top of
it. Confirms it if the existing response shape renders directly.

## Acceptance criteria

- [ ] Corrupting one entry (e.g. alice-demo +5.00) makes a drift table appear
      below the verdict, naming alice-demo with its stored balance, computed
      balance, and a delta of 5.00
- [ ] Corrupting a second account (e.g. bob's stored balance +7.00) adds a
      second row for bob without altering alice-demo's row
- [ ] A healthy account (e.g. bob, when only alice-demo is drifted) never
      appears in the drift table
- [ ] A healthy ledger shows no drift table at all
- [ ] `make demo-console-02` shows the drift table appearing and disappearing
      correctly across a healthy → corrupted → healthy cycle, run by hand

## Data

Real, reused from `ledger-core`'s demo evidence: `alice-demo` altered by
5.00, `bob` altered by 7.00 (both exact figures from the parent feature's
Post-Merge Integration Gate scenarios).

## Dependencies

Slice 01 (page shell and verdict fetch already in place; this slice extends
the same response's `drifted` field).

## Effort

≤1 day. Reference class: one table component driven by an array already
present in the slice-01 fetch response — no new network call.
