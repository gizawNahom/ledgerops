# Slice 03 — Idempotent retry

**Job**: J2 | **Invariant**: I7 | **Story**: US-3 | **WS strategy**: C (real local)

## Goal

Make the same transfer request safe to submit twice: it posts once, and the
second submission returns the original result.

## Learning hypothesis

Disproves that idempotency can be bolted on after the write path exists. If the
key has to be stored transactionally alongside the transaction it guards — and it
does — then retrofitting it means changing the write path built in slice 01.
Discovering that now is cheap; discovering it after five more slices is not.

## IN scope

- Caller-supplied `Idempotency-Key` header on `POST /transfers`
- Key stored in the same database transaction as the transaction it guards
- Replay returns the original response, not a fresh posting
- Conflict detection: same key, different payload, returns 409 rather than
  silently replaying the original
- Retry demo: same key submitted 50× concurrently, one entry pair results

## OUT of scope

- Key expiry or garbage collection (note it as a known gap; unbounded growth is
  acceptable at this stage but must be recorded)
- Idempotency on any endpoint other than `POST /transfers`
- Client-side retry helpers or SDKs

## Acceptance criteria

- [ ] Same key submitted twice posts exactly one transaction (I7)
- [ ] Second submission returns the same `transaction_id` as the first
- [ ] Same key with a different payload returns 409 `idempotency_key_conflict`
- [ ] Key and transaction commit atomically — killing the process between them is
      not possible because they are one write
- [ ] 50 concurrent submissions of one key yield exactly one transaction
- [ ] Missing key on `POST /transfers` is rejected, not silently permitted —
      optional idempotency is idempotency nobody uses
- [ ] Property-based test: for any generated request and any repeat count ≥1,
      final state equals the state after exactly one application
- [ ] `make demo-03` shows the replay; `make race-03` runs the concurrent test

## Data

Synthetic, by documented exception (D6).

## Dependencies

Slice 01 (write path). Slice 02 is not strictly required but is assumed by the
ordering, since isolation is already settled by then.

## Effort

≤1 day. Reference class: a uniqueness constraint plus a stored response body. The
concurrency test is again the time sink.

## Changed Assumptions

**Original assumption** (this file, DISCUSS wave, 2026-08-17):

> Whether the stored replay response is the full response body or a reference that
> is re-rendered. Storing the body is simpler and immune to later format drift;
> re-rendering keeps storage small but can return a response the caller never
> originally saw. Recommend storing the body. Flag for the architect.

**New assumption** (DESIGN wave, DDD-8, `adr-005-idempotency-replay.md`):

The idempotency record stores key + request fingerprint + `transaction_id`, and
replays are **re-rendered from the stored transaction**. Response bodies are not
cached.

**Rationale for the change**: the original recommendation was written before D7
(append-only entries) was locked. Without immutability, a re-render could differ
from what the caller originally received, so caching the body was safer. With D7
in force the transaction cannot change, making re-rendering deterministic — and
cached bodies acquire a drawback, since a later response-format change would
leave replays serving the old shape indefinitely.

**Consequent AC change**: the acceptance criterion below asserting the second
submission returns the same `transaction_id` is unaffected. Add: response
rendering must be a pure function of the stored transaction, and the fingerprint
must be computed over canonical JSON so key ordering or whitespace cannot
produce a false 409.
