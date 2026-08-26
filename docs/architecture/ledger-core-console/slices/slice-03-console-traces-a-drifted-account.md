# Slice 03 — Console traces a drifted account to its entries

**Job**: J5 | **Story**: US-3 | **WS strategy**: inherits `ledger-core` Strategy C; rendering itself is manual dogfood

## Goal

Operator clicks a drifted `account_id` and sees its ordered entries with a
running balance, making the exact point of divergence visible.

## IN scope

- Click handler on a drift-table row (or a directly-navigated `account_id`)
  that fetches `GET /accounts/{id}/entries`
- Render the response as an ordered table: amount, counterparty,
  `recorded_at`, running balance — using the fields the endpoint already
  returns, no client-side recomputation
- Divergence is visible because the running balance column itself diverges
  at the tampered row; no extra highlighting logic required to satisfy the
  underlying contract, though visual emphasis (e.g. a colored row) is
  reasonable polish

## OUT scope

- Fetch-failure handling (slice 04)
- Editing, correction, or repair actions from this view
- Comparing two accounts' traces side by side

## Learning hypothesis

Disproves "the `running_balance` field from `GET /accounts/{id}/entries` is
enough on its own to make a divergence visually obvious, with zero SPA-side
computation" if the SPA turns out to need extra logic (e.g. computing an
expected balance itself) to make the break point legible. Confirms it if a
plain ordered table already shows the divergence, exactly as the parent
feature's own demo output already showed via curl (running balance 105.00
vs. stored balance 100.00, "diverging visibly at the offending row").

## Acceptance criteria

- [ ] Clicking alice-demo's row in the drift table (slice 02) shows
      alice-demo's entries, ordered, with a running balance column
- [ ] The running balance visibly diverges starting at the entry altered
      out-of-band
- [ ] Each row also shows counterparty account and `recorded_at`
- [ ] Opening a healthy account's trace (e.g. bob's, when not drifted) shows
      a running balance that matches the account's stored balance throughout
- [ ] A failed or timed-out entries fetch (after clicking a drifted row)
      shows a bounded-time error naming the exact `GET /accounts/{id}/entries`
      URL as a direct fallback — no blank panel, no indefinite spinner
      (added: comprehensive Phase 2 redo, 2026-08-25 — this is a materially
      different, higher-stakes failure than US-4's verdict-fetch failure,
      since it strikes after the operator is already alarmed and
      mid-investigation; see `discuss/journey-console-visual.md`, "S4-fail")
- [ ] `make demo-console-03` clicks through from a corrupted drift row to the
      entry trace and confirms the divergence is visible, run by hand

## Data

Real, reused from `ledger-core`'s demo: alice-demo funded 100.00 by
treasury-demo (`txn_1b575c10-...`), entry altered by 5.00 out-of-band,
running balance 105.00 diverging from stored balance 100.00 — the exact
figures already produced by the parent feature's `curl /accounts/alice-demo/entries`
demo.

## Dependencies

Slice 02 (drift table provides the `account_id` to click through). The
underlying endpoint (`GET /accounts/{id}/entries`) is already delivered and
contract-tested by `ledger-core` slice 05.

## Effort

≤1 day. Reference class: one table component driven by an existing endpoint
response, plus one click handler wiring the account_id from slice 02's table
into a second fetch.
