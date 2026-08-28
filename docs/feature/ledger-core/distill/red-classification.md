# RED classification — ledger-core

Pre-DELIVER fail-for-the-right-reason gate (`nw-distill`). DELIVER reads this at
PREPARE to confirm RED is genuine. First recorded 2026-08-18 as PARTIAL;
**executed and rewritten 2026-08-19** once a Docker daemon was available.

Command: `LEDGEROPS_AT_TAGS="" go test ./tests/acceptance/ledgercore/...`
Suite as executed: 7 `.feature` files · 54 scenarios (53 blocks) · 396 step
invocations. Every count in this document is the count as executed on
2026-08-19; see § Scenarios added after this run.
Environment: Docker 29.7.2 (Ubuntu 26.04, overlayfs) · testcontainers-go v0.33.0
· `postgres:16` · run wall time 2m29s.

## Result: PASS — RED is genuine, handoff to DELIVER is not blocked

The gate's verdict is binary by construction (`nw-distill` § Pre-DELIVER
fail-for-the-right-reason gate, step 3): **BLOCK if any scenario classifies as
category 2 or 3, otherwise pass.** Zero scenarios classify as category 2 or 3,
so the verdict is PASS, unqualified. It is not "conditional" — the gate has no
conditional band and inventing one would leave DELIVER unsure at PREPARE whether
it may start. It may.

Two obligations are carried forward to DELIVER — a second gate run
(§ DELIVER's obligation) and a standing warning against counting the vacuous
pass (§ Caveats). Neither qualifies this verdict; both are additional work
assigned on the strength of it.

### The classification rule this verdict applies

Stated up front because the verdict turns on it, and because a different rule
gives a different answer.

**Scenarios are classified by the _cause_ of failure, not by the _depth_ the
scenario reached before failing.** A scenario is RED when the thing that made it
fail is missing production code. It does not have to have executed its `Then` to
qualify.

This is what the mandates say, not a local preference. Mandate 7 (RED-ready
scaffolding) requires scaffolds to raise an **assertion** failure precisely so
the Red Gate Snapshot classifies by error *type*: assertion → RED, and
`ImportError` / `NotImplementedError` / module-missing → BROKEN. The gate's
wrong-RED category is enumerated by cause — `IMPORT_ERROR`, `FIXTURE_BROKEN`,
`SETUP_FAILURE` — and a scaffold assertion firing inside a `Given` is none of
those three. Every one of the 48 shallow failures is a scaffold assertion. They
are category 1.

The competing reading — RED requires the assertion itself to run and fail —
would put this gate at PARTIAL, with only 5 of 53 qualifying. It is recorded
here rather than hidden, for two reasons. First, it is the reading a careful
person arrives at independently. Second, it is unsatisfiable by construction
under Mandate 1: port-to-port seeding forbids back doors, so on a greenfield
feature no scenario can reach its assertion until the first slice exists, and a
depth-based gate could therefore never pass before DELIVER — which is exactly
when it is meant to be read. A rule that can only ever return PARTIAL carries no
information. **Anyone applying the depth rule should read this gate as PARTIAL
and the § RED depth table as the reason.**

```
54 scenarios (1 passed, 53 failed)
396 steps (124 passed, 53 failed, 219 skipped)
```

Every one of the 53 failures carries an `__SCAFFOLD__` marker in its message.
Not one failure is a container, connection, binding, or fixture fault.

| Class | Count | Meaning |
|---|---|---|
| `MISSING_FUNCTIONALITY` | 53 | failure caused by unimplemented production code — correct RED |
| `SETUP_FAILURE` / `FIXTURE_BROKEN` | **0** | no scenario failed for a harness reason |
| `BROKEN` (transport / panic / undefined step) | **0** | no dropped connections, no panics escaping a handler, no unbound steps |
| PASS on first run | 1 | `The schema builds from nothing` — **vacuous**, see § Caveats |

The four distinct failure messages across all 53:

```
50 ×  expected the transfer to be accepted, but it was refused
        (answer: {"error":"__SCAFFOLD__","operation":"post transfer", ...})
 1 ×  ... {"error":"__SCAFFOLD__","operation":"create account", ...}
 1 ×  expected a refusal for unknown_account, got __SCAFFOLD__  (post transfer)
 1 ×  expected a refusal for unknown_account, got __SCAFFOLD__  (trace entries)
```

Both scaffold design choices did their job. HTTP handlers answered `501` with a
`__SCAFFOLD__` body rather than panicking, so no scenario died in transport.
`postgres.Migrate` and `postgres.Open` no-opped rather than panicking, so no
scenario died establishing its store. Both were made specifically to prevent
wrong-RED, and the run confirms they work.

## RED depth — read this before using the suite as a guide

The 53 correct failures are not equally deep. Only **5 reach their `Then`**; the
other **48 fail while establishing a `Given`**:

| Where the scenario failed | Count | What it proves |
|---|---|---|
| At a `Then` — assertion executed and unmet | 5 | full RED: the assertion runs and fails for the right reason |
| At a `Given`/`And` — seeding blocked | 48 | correct cause, but the assertion has not yet executed |

The 5 that reach their assertion:

- `A new account starts empty` (milestone-01:18)
- `Value enters the ledger only as a movement from the system account` (milestone-01:24)
- `A transfer out of an account nobody opened is refused and nothing moves` (milestone-01:62)
- `A system account is permitted to go negative` (milestone-02:52)
- `Tracing an account nobody opened is refused` (milestone-05:62)

**This is expected, not a defect — it is Mandate 1 working.** Hexagonal boundary
enforcement admits no back-door seeding: `GivenFundedAccount` funds an account by
calling the real `POST /transfers`, exactly as an operator would. Seeding through
the store directly would make these 48 scenarios reach their `Then` today, and
would be a Mandate 1 violation and Testing Theater — a suite that proves the
assertions run while proving nothing about whether a user can reach them. The
shallow depth is the price of the correct boundary, and it is self-clearing: the
moment slice 01 lands, the seeding calls succeed and those 48 failures migrate
from the `Given` to the `Then` on their own, with no test edit.

## DELIVER's obligation — a second gate run

This is a required step in DELIVER, not a suggestion and not a nice-to-have.

All 396 step invocations are bound — godog reported a definition location for
every step, including skipped ones, so there are no unbound steps. But the
*bodies* of the assertions in those 48 scenarios have not executed. A logic
error inside an assertion method would not have surfaced yet.

**After the walking skeleton goes green, re-run the full suite with the tag
filter disabled and confirm two things: the correct-RED count holds, and the
failure messages for those 48 scenarios now come from `Then` clauses rather than
`Given` clauses.** Any scenario that still fails in a `Given` after slice 01 is
green is a test defect and must be fixed before that slice is committed.

## Scenarios added after this run

The suite on disk is now **59 blocks / 60 executed**. Six scenarios were added
later on 2026-08-19 by the AT completeness audit (`feature-delta.md` § AT
completeness audit): the smallest accepted amount, a key surviving an
interruption, an empty ledger's verdict, two damaged accounts, an empty trace,
and a single-row trace.

**The verdict above stands exactly as recorded.** All six are `@pending` and
none has ever executed, so none can have changed a classification — no scenario
that never ran can turn a `MISSING_FUNCTIONALITY` into a `SETUP_FAILURE`. The
53 / 54 / 396 figures throughout this document are the counts as executed on
2026-08-19 and are correct as such; they are not the size of the suite today.

Classifying the six is part of the second gate run required above — the run must
cover all 60 and confirm each of the six fails for a missing implementation
rather than a broken `Given`, on the same rule stated in § The classification
rule this verdict applies.

## Caveats

**The one PASS is vacuous.** `The schema builds from nothing` passes because
`postgres.MigrationStatements()` returns an empty map, so "no migration in the
set erases an entry" is true of the empty set. It is not evidence of anything.
DELIVER must not read that green as coverage; it becomes meaningful only once
migration 0 exists.

**Two predictions in the 2026-08-18 draft were wrong**, recorded here rather than
quietly corrected, because the draft was written without a Docker daemon and its
numbers were estimates presented as expectations.

| Predicted 2026-08-18 | Measured 2026-08-19 |
|---|---|
| 51 scenarios reach their `Then` | 5 |
| the 3 unauthenticated-caller scenarios pass on first run | 0 pass |

Neither miss changes the verdict — both are about depth, and depth does not
enter the classification rule. But the second one retracts a claim: the draft
implied the API-key middleware was proven by this run. It is not. Each of the 3
auth scenarios seeds an account first, so it dies in its `Given` before the
middleware is ever reached. The middleware is real and unscaffolded, and this
run says nothing whatsoever about whether it works. Verifying it moves to
DELIVER, and the second gate run is where it gets verified.

**What this run still does not prove.** `postgres.Open` and `Migrate` are
no-ops, so no pgx connection was ever opened. Untouched by this gate: every SQL
path, the two-role split (`ledgerops_app` versus `ledgerops_migrate`, OPS-10),
row-lock ordering, and privilege revocation. The container starts and is
reachable; nothing has talked to it yet.

## Verified alongside

| Check | Result |
|---|---|
| Suite compiles | PASS — `go build ./...` clean |
| `go vet` over suite and production | PASS — no findings |
| Zero undefined steps | PASS — 396/396 bound, tag filter disabled |
| Scaffolds present for every imported production module | PASS — 9 files carry `SCAFFOLD: true` |
| Walking skeleton runs in isolation | PASS — default tag filter selects it alone, fails on `__SCAFFOLD__` |
| Testcontainers lifecycle | PASS — ryuk + `postgres:16` start and stop per scenario, 54 times, no leaks |

---

## Addendum — OPS-5 observability fix (2026-08-26)

Gate re-run, scoped to the new `milestone-06-observability.feature`, not the
whole suite: `LEDGEROPS_AT_TAGS="@ops-5" go test -count=1 -v
./tests/acceptance/ledgercore/...`. Run three times over the course of this
DISTILL pass; the first two runs found and fixed real defects in the test
authoring itself (below). This section reports the third, clean run.

```
23 scenarios (6 passed, 17 failed)
149 steps (125 passed, 17 failed, 7 skipped)
```

### Result: PASS — RED is genuine, handoff to DELIVER is not blocked

Same binary verdict rule as the original gate above. Zero scenarios classify
as category 2 (`IMPORT_ERROR`/`FIXTURE_BROKEN`/`SETUP_FAILURE`) or category 3
(`WRONG_ASSERTION`/`OBSERVABLE_NOT_AT_PORT`).

| Class | Count | Meaning |
|---|---|---|
| `MISSING_FUNCTIONALITY` | 17 | correct RED — every failure traces to unbuilt production behaviour |
| `SETUP_FAILURE` / `FIXTURE_BROKEN` | **0** | none |
| `BROKEN` (undefined step / panic / transport fault) | **0** | none |
| PASS (legitimate, pre-existing) | 6 | the regression `Scenario Outline`'s six examples — `requireOperatorKey` is real, unscaffolded production code from slice 01, so these were never expected to fail. Not vacuous: see § Vacuous-pass check below |

The 17 failure messages, deduplicated:

```
8 ×  no log line was captured for the last request
2 ×  the exposition carries no ledgerops_drift_accounts series to read
2 ×  expected the scrape to succeed with status 200, got 401
       (body: {"error":"unidentified_caller"})
1 ×  the exposition carries no ledgerops_trial_balance_imbalance_minor series to read
1 ×  the exposition carries no ledgerops_postings_total{result="replayed"} series to read
1 ×  the exposition carries no ledgerops_postings_total{result="rejected"} series to read
1 ×  the exposition carries no ledgerops_postings_total{result="posted"} series to read
1 ×  both requests must have produced a captured log line to compare their routes
```

8+2+2+1+1+1+1+1 = 17, reconciling exactly against the scenario count. Every
message traces to one of two absences DELIVER must build: no request-logging
middleware is registered (8 log-line failures + the route-comparison failure
= 9), or `GET /metrics` is still the pre-existing `501 __SCAFFOLD__` route,
both because no series exist yet (6 exposition-parsing failures) and because
it still sits inside `requireOperatorKey` (2 "got 401" failures, the router
restructuring this fix requires has not happened). None of the 17 dies for a
harness reason — no container fault, no JSON-decode panic, no nil pointer.

### Two real defects this gate run caught and fixed (why three runs, not one)

The pre-DELIVER gate exists to catch exactly this class of thing before
DELIVER starts, not just to rubber-stamp a green build. Both were caught by
running the suite for real rather than reasoning about it statically.

1. **2 undefined steps, run 1.** The release-blocking scenario's identity
   switches ("When ... And the caller presents an operator key that was
   never issued") reported undefined, even though the phrasing was already
   registered — as a `Given`. godog's keyword matching is scoped to the
   registering function, and `And` inherits the keyword of the nearest
   preceding `Given`/`When`/`Then`; a `Given`-only registration does not
   answer an `And` inheriting `When`. Fixed by registering the same two
   phrasings again under `ctx.When` in `steps_ledger_test.go`, delegating to
   the identical behaviour. Confirmed: run 2 reported 0 undefined.
2. **1 vacuous pass, run 2.** "The operator's credential never appears in a
   captured log line, however the caller is answered" passed on its first
   real run — not because redaction works, but because **nothing is logged
   at all yet**, so a secret trivially cannot appear in an empty capture. A
   security scenario that can pass before the feature exists is the same
   trap as `distill/red-classification.md`'s original "schema builds from
   nothing" vacuous pass, and worse here because this is the
   `@release-blocking` scenario the task brief made mandatory. Fixed by
   adding a positive companion assertion ("Then a log line for that request
   carries a request id, its route, its status, and how long it took") ahead
   of the negative one, so the scenario cannot pass until real logging
   exists — confirmed: run 3 reports this scenario as `MISSING_FUNCTIONALITY`
   (`no log line was captured for the last request`), not a pass.

**DELIVER must not read the 6 outline passes as OPS-5 coverage** — they
prove the pre-existing `requireOperatorKey` middleware still works, which
this fix does not touch except to move one route (`GET /metrics`) out of its
group. If any of those 6 examples ever fails after DELIVER's router
restructuring, that is a regression this fix introduced, and it blocks the
slice.

### Verified alongside (OPS-5 scope)

| Check | Result |
|---|---|
| Suite compiles | PASS — `go build ./...` clean |
| `go vet` over suite and production | PASS — no findings |
| Zero undefined steps | PASS — confirmed on run 2 and run 3 |
| Scaffolds present for both new production symbols | PASS — `Deps.Logger` field (`router.go`) and `request_logging.go`'s `requestLogger` both exist, neither panics |
| Pre-existing GREEN unaffected | PASS — 6 regression-outline examples pass exactly because slice 01's auth middleware is real, not because of anything this fix built |
