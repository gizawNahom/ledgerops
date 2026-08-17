# Slice 05 — Entry traceability

**Job**: J5 | **Invariant**: none (supports I3 investigation) | **Story**: US-5
**WS strategy**: C (real local)

## Goal

Let the operator drill from a balance into the ordered entries that produced it,
with a running balance that makes the break point visible.

## Learning hypothesis

Disproves that the entry model carries enough context to explain a balance. If an
operator can see every entry and still cannot say *why* a balance is what it is,
the entries are missing fields — description, counterparty, source transaction —
and that is a schema problem worth finding before more history accumulates under
the wrong shape.

## IN scope

- `GET /accounts/{id}/entries` — ordered entries for one account
- Running balance column, computed cumulatively down the list
- Each entry shows its transaction id, counterparty account, amount, and timestamp
- Console: clicking an account from the slice-04 drift list opens this view
- Stable ordering — entries must sort deterministically, including entries
  created within the same clock tick

## OUT of scope

- Filtering, search, date ranges, or pagination beyond a simple limit
- Export (CSV, JSON download)
- Cross-account views or full transaction browsing
- Editing or annotating entries — D7 forbids mutation

## Acceptance criteria

- [ ] `GET /accounts/{id}/entries` returns entries in deterministic order
- [ ] Running balance on the final row equals the account's stored balance on a
      healthy account
- [ ] On a corrupted account (from slice 04), the running balance visibly
      diverges from the stored balance, and the row where it diverges is the
      corrupted entry
- [ ] Each entry names its counterparty account, so the movement is legible
      without cross-referencing the transaction table by hand
- [ ] Two entries created in the same clock tick still order deterministically
- [ ] Clicking a drifted account in the console reaches this view
- [ ] `make demo-05` traces a healthy account; `make trace-05` traces the
      corrupted one from slice 04

## Data

Synthetic, by documented exception (D6).

## Dependencies

Slice 01 (entries). Slice 04 (the drift list this links from, and the corrupted
account that makes the tracing demo meaningful).

## Effort

≤1 day. Reference class: one indexed query plus a table view. The deterministic
ordering requirement is the only subtle part.

## Note

This slice closes the operator loop opened in slice 04: 04 says *something is
wrong*, 05 says *here is exactly what*. Demoed together they tell one story —
detection followed by diagnosis — which is more compelling than either alone.
