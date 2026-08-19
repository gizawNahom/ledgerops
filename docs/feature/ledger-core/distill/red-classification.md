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
