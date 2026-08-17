# ADR-005 — Idempotency replays re-render from the stored transaction

**Status**: Accepted · 2026-08-18 · Feature: ledger-core · Decision: DDD-8
**Supersedes**: the recommendation recorded in `slices/slice-03-idempotent-retry.md`

## Context

I7 requires that applying the same transfer request twice changes state only
once, and that the second call returns the same answer as the first. The open
question was what to persist in order to answer a replay.

## Decision

The idempotency record stores three things: the **key**, a **fingerprint of the
request payload**, and the resulting **transaction_id**. A unique constraint on
the key, written inside the posting transaction, is what makes I7 hold under
concurrency.

On replay, the response is **re-rendered from the stored transaction** rather
than served from a cached body.

Behaviour:

| Situation | Response |
|---|---|
| New key | Post, store key + fingerprint + transaction_id, return 201 |
| Same key, same fingerprint | Re-render from transaction_id, return the original result |
| Same key, different fingerprint | 409 `idempotency_key_conflict` |
| Missing key | 400 — the header is required, not optional |

## Alternatives considered

**Store the full response body** — this was the recommendation written into the
slice-03 brief during DISCUSS, and it is reversed here.

At the time it was written, D7 (append-only entries) had not yet been decided.
Without immutability, re-rendering could produce a different answer than the
caller originally received, and caching the body was the safer choice. With D7
locked, the transaction cannot change, so re-rendering is deterministic — and
storing bodies acquires a real drawback: after any change to response format,
replays would serve the old format indefinitely, so two callers could see
different shapes for equivalent requests.

Recorded explicitly because reversing a documented recommendation without
explanation is worse than the original error.

**Key alone, no fingerprint** — simpler, but silently replays the original result
when a caller reuses a key for a genuinely different transfer. Failing loudly
with 409 is far better than quietly not doing what was asked.

**Optional idempotency key** — rejected. An optional safety mechanism is one most
callers will omit, and the retry hazard is the default condition of any network,
not an edge case.

## Consequences

- The idempotency record and the transaction commit together. There is no window
  in which one exists without the other.
- Key storage grows without bound. Expiry is a known, documented gap — noted in
  slice 03's OUT-of-scope list, not silently ignored.
- Response rendering must be a pure function of the stored transaction. That is
  a constraint on the HTTP adapter, and worth a test.
- The 409 path needs the fingerprint to be stable — canonical JSON, not raw bytes,
  so that key ordering or whitespace does not produce a false conflict.
