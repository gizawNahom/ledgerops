# Slice 04 — Proof of balance

**Job**: J4 | **Invariant**: I3 | **Story**: US-4 | **WS strategy**: C (real local)

## Goal

Give the operator a single view that answers "do the books balance?" — and that
turns red when they do not.

## Learning hypothesis

Disproves that correctness is self-evident. A ledger that is correct but cannot
demonstrate correctness does not resolve the anxiety force ("trusting money to
unproven code"). If the console cannot detect a hand-corrupted row, the
verification model is wrong.

## IN scope

- `GET /console` — operations console, operator API key required
- Verdict first: a stated sentence, "Books balance: YES / NO", not a table the
  operator must compute from
- Trial balance: sum of all entries, which must be zero
- Per-account reconciliation: stored balance vs. balance computed from entries,
  listing any account where they disagree (I3)
- `GET /health/trial-balance` — same verdict via API, so the console is not the
  only path to it. Response includes `entry_count` and `elapsed_ms`
- Corruption demo: modify an entry directly in the database, refresh, watch the
  console go red and name the account

## OUT of scope

- Drill-down into individual entries (slice 05)
- Alerting, notification, or scheduled checks
- Repair tooling — detection only; fixing is manual and deliberate at this stage
- Any customer-facing view
- **Checkpointing / incremental verification** — deferred by user decision
  2026-08-17. Full scan is correct and adequate at current volume. The
  `elapsed_ms` instrumentation exists so degradation is observed rather than
  guessed at. D7 (append-only) keeps the checkpointing option open.

## Verification approach

Full scan: sum all entries, group by account, compare against stored balances.
O(total history) and deliberately so. This is the simplest thing that is
actually correct, and correctness is the point of the slice.

Because the corruption demo modifies a *historical* entry, incremental
verification would miss it — any future checkpointing design must pair with
tamper evidence (hash chaining) rather than replacing the full scan outright.
Recorded here so the trade-off is not rediscovered later.

## Acceptance criteria

- [ ] Console states the verdict in words before showing any figures
- [ ] Trial balance across all entries equals zero on a healthy ledger
- [ ] Console shows green when consistent, red when not
- [ ] Corrupting one entry's amount causes the console to report NO and name the
      affected account with stored, computed, and delta
- [ ] Performing that corruption requires deliberately defeating the D7
      append-only constraint — the demo documents the step taken
- [ ] `GET /health/trial-balance` returns the same verdict as the console
- [ ] `GET /health/trial-balance` reports `entry_count` and `elapsed_ms`
- [ ] Console requires the operator API key; unauthenticated access is refused
- [ ] `make demo-04` shows a green console; `make corrupt-04` performs the
      corruption and shows it caught

## Data

Synthetic, by documented exception (D6). The corruption test is deliberately
hand-crafted rather than generated — it models a specific adversarial event.

## Dependencies

Slice 01 (entries to check, and the D7 constraint to defeat). Slices 02 and 03
are not required.

## Effort

≤1 day. Reference class: one aggregate query plus one page in the TypeScript SPA
(DDD-4 / ADR-006 — the console is a separate SPA, not server-rendered; wording
corrected 2026-08-18, DISTILL finding DDR-2). Resist building a dashboard
framework; this is one page with one verdict. ADR-006 already flags that the
second toolchain puts this slice's one-day ceiling at risk — that risk is the
reason to keep the page this small.

## Note

This slice carries the highest signal-to-effort ratio in the feature. A system
that detects its own corruption is what a ledger is *for*. Prioritise the
corruption demo's polish over the happy-path view.
