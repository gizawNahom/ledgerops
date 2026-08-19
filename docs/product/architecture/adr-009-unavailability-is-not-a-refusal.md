# ADR-009 — An unreachable store is not a refusal, and the verdict withholds rather than accuses

**Status**: Accepted · 2026-08-19 · Feature: ledger-core · Decisions: DDD-20, DDD-21
**Relates to**: ADR-008 (the sealed refusal set), ADR-005 (idempotent retry), OPS-1, OPS-10
**Closes**: AT-completeness gap C7a, DESIGN half

## Context

Nothing declares what the ledger answers when PostgreSQL is unreachable, the
connection pool is exhausted, or a write fails on a full disk. The completeness
audit routed it to DEVOPS and DESIGN jointly. DEVOPS owns the environment it
would be exercised in; DESIGN owns the prior question, which is whether an
unreachable store is a member of the sealed refusal taxonomy at all. The status
code follows from that and not the other way round.

## Decision

**Unavailability is categorically outside the sealed set** (DDD-20).

A member of the taxonomy is *the rules saying no*. It is a decision: the ledger
read the world, applied its rules, and declined. A store that is gone produces
no decision. The ledger has not refused the transfer — it does not know whether
the transfer happened.

The second reason is specific to this feature and is the sharper one. Every
refusal carries an implicit assertion about state, which the acceptance suite
makes explicit by tagging all of them `@contract-shape:unbounded-preservation`:
the action mutated nothing. A store failure cannot honour that. A connection
lost after `COMMIT` was sent and before the acknowledgement arrived may have
mutated everything. Classifying it as a refusal would attach a guarantee the
system is unable to make, and would do it inside the one contract shape the
project relies on to keep read paths from writing.

So the answer to the caller is not "refused". It is "I do not know — retry",
and ADR-005 already makes retrying safe.

**Answers** (DDD-20):

| Condition | Status | Body | Header |
|---|---|---|---|
| Store unreachable, connection refused, statement or lock timeout | **503** | `{"error":"service_unavailable"}` | `Retry-After: 1` |
| Connection pool exhausted | **503** | same | `Retry-After: 1` |
| Write fails on a full or read-only volume | **503** | same | `Retry-After: 1` |

`service_unavailable` is **not** a `RefusalKind` and **not** a
`domain.ViolationKind`. It is an availability outcome, carried in the same error
envelope so callers parse one body shape, and deliberately absent from both
sealed sets — there is no switch over it, so DDD-12's `exhaustive` obligation
does not extend to it.

**The verdict withholds; it never accuses** (DDD-21). When the store is
unreachable, `GET /health/trial-balance` answers **503** and the `verdict` field
is **absent**. It must not read `Books balance: NO`.

*Absent* means **omitted, not `null`**, and the whole verdict envelope goes with
it: a 503 body is exactly `{"error":"service_unavailable"}` — no `verdict`, no
`imbalance_minor`, no `entry_count`, no `elapsed_ms`. `null` is a value and
would say "I have a verdict and it is nothing", which is the ambiguity this
decision exists to remove. On 200, `verdict` is always present and never null,
so its absence is conclusive proof the ledger was not read. The console branches
on the status code, never on the field.

`NO` means "I looked, and the books do not balance". Returning `NO` because the
database is down is a false accusation of corruption. It would also silently
destroy the KPI-4 signal that slice 04 exists to produce: a corruption-detection
gate that fires on an unreachable database no longer measures corruption
detection. The console surface must render this as a third state — *cannot reach
the ledger* — distinct from both YES and NO. The `verify-the-books` journey
already declares the fallback when the *console* is unreachable; this declares
the answer when the *ledger* is.

**No in-request retry, and no circuit breaker** (DDD-20). The posting path fails
fast. A retry inside the request would be a second database transaction with a
second lock acquisition, which is a new posting under DDD-6, not a repeat of the
first — and the caller already holds the correct retry primitive in the
idempotency key. A circuit breaker is rejected for now on merit: there is one
dependency and nothing downstream of it to protect from cascade, and OPS-1
records that there is no traffic. Recorded as rejected rather than omitted, so
that its absence reads as a decision.

**Wire, then probe, then use.** The composition root (`cmd/api/`) probes the
store before the server accepts a connection, and refuses to start if the probe
fails, emitting a structured `health.startup.refused` event. The probe asserts
two things, not one:

1. The app-role connection can open a transaction and read.
2. `UPDATE` on the entry table as `ledgerops_app` is **refused** (OPS-10).

The second is the one worth arguing for. CI job 6 asserts the revocation on
every push, which proves the migration set is right; it proves nothing about the
database the binary is actually pointed at. A privilege re-granted by hand, a
DSN pointing at the migrate role, a restored snapshot predating the role
split — each yields a running service whose append-only guarantee is discipline
in a database costume, and each is invisible until the day it matters. A
guarantee nobody checks at wiring time is a guarantee that quietly disappears.

## Alternatives considered

**Add `store_unavailable` to the sealed taxonomy** — appealing because it keeps
one vocabulary and one exhaustive switch. Rejected: it makes DDD-12's promise
false. The set would stop being a set of answers about the *ledger* and become
one containing a single answer about the *service*, and every consumer that
reasons "a refusal means nothing moved" would be wrong exactly once, in the case
where being wrong costs the most.

**429 for pool exhaustion** — considered seriously and rejected. An exhausted
pool is a property of the service's capacity, not of the caller's rate. There is
one caller, no rate policy, and no throttling designed. If a rate policy is ever
introduced, 429 becomes the right answer for *that*, and 503 stays right for
this. Splitting them now would give the caller a distinction it cannot act on.

**500 rather than 503** — rejected. 500 says "I am broken"; 503 says "try again
shortly", which is both true and actionable, and `Retry-After` carries the only
advice worth giving.

**`Books balance: NO` when the store is unreachable** — rejected, and it is the
alternative most likely to be implemented by accident, because "the check did
not succeed" collapses so naturally into "the check failed". The distinction
between *the books are wrong* and *I could not look* is the whole product value
of slice 04.

**Retry with exponential backoff inside the posting path** — rejected. Under
DDD-6 a retry is a fresh lock acquisition and a fresh transaction, so it
lengthens the window in which locks are held under exactly the contention the
race suite exists to characterise. The idempotency key moves the retry to the
caller, where it is already safe and already tested.

## Consequences

- A `degraded` environment does not exist in `devops/environments.yaml`. It is
  now required, and the DESIGN-side preconditions are named in
  `feature-delta.md` § Store unavailability. Owner: DEVOPS.
- Every driving port acquires a status the audit did not cover: 503. It is
  reachable on all six of them, since all six touch the store.
- The startup probe means a misconfigured deployment fails loudly at boot rather
  than serving 503s from a binary that never had a database. Cost, accepted: the
  service will not start against a database whose role split is wrong, including
  a developer's hand-made local database. That is the intended failure.
- The observability contract needs a series for unavailability and a log field
  distinguishing 503 from a refusal, so that a spike in `503` is never counted as
  a spike in rejections. Owner: DEVOPS; raised, not edited here.
- `service_unavailable` deliberately does not appear in the acceptance suite's
  `RefusalKind`. A scenario asserting it asserts an outcome, not a refusal, and
  the distinction is what keeps `@error` a refusal taxonomy.
