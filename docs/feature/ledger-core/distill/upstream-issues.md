# Upstream issues found during DISTILL — ledger-core

Back-propagation record. Findings that belong to a prior wave's artifact, raised
here because writing the scenarios is what surfaced them. Recorded 2026-08-18.

Three were resolved by user ruling before any scenario was written (DDR-1..3
below); two more were raised while writing them (U-1, U-2). All five are now
closed — each upstream edit was made only after the owning wave's reviewer
confirmed the finding and directed the change. Two further findings came out of
the review cycle itself and are recorded at the end: R-1 (closed) and **R-2**
— a HIGH reviewer condition the user chose on 2026-08-19 to record and defer
rather than implement before DELIVER. **R-2 closed 2026-08-21 at DELIVER step
05-04**, per its own entry below.

---

## Resolved before scenario writing

### DDR-1 — `Idempotency-Key` required from slice 03, but US-1's pitch omits it

**Where**: `feature-delta.md:57` (US-1 elevator pitch) versus
`adr-005-idempotency-replay.md` ("Missing key | 400 — the header is required,
not optional") and slice-03's AC.

**The conflict**: US-1's "After" line is `curl -X POST /transfers -d '{...}'`
with no key. Once slice 03 lands, that exact command returns 400. The pitch
therefore describes an interaction the finished system refuses.

**Ruling** (user, 2026-08-18): every `POST /transfers` scenario carries a key
from slice 01 onward. Slice 01 ignores it, slice 03 gives it meaning. No
slice-01/02 scenario needs rewriting when slice 03 lands.

**Upstream fix applied 2026-08-18**, after the DISCUSS reviewer confirmed the
finding and directed the change: `feature-delta.md:57` now reads
`curl -X POST /transfers -H 'Idempotency-Key: <caller-generated>' -d '{...}'`.
DISTILL did not make this edit unilaterally — the review gate is what authorised
touching another wave's story text. **CLOSED.**

### DDR-2 — the console is an SPA in DESIGN and a server-rendered page in DISCUSS

**Where**: `feature-delta.md:166` lists `GET /console` as a Web UI driving port
and `slices/slice-04-proof-of-balance.md` § Effort calls it "a minimal
server-rendered page"; DDD-4 / ADR-006 make it a separate TypeScript SPA.

**Compounding evidence**: the DEVOPS eight-job pipeline
(`feature-delta.md:447`) has no browser job. Whatever the console is, nothing in
CI drives it as a user would.

**Ruling** (user, 2026-08-18): console scenarios assert the verdict contract
over HTTP; browser E2E is deferred. The SPA rendering is recorded as a known
untested seam in `docs/architecture/atdd-infrastructure-policy.md` § Known gap.
CI stays at eight jobs.

**Upstream fix applied 2026-08-18**, confirmed by both the DISCUSS and DESIGN
reviewers: `slices/slice-04-proof-of-balance.md` § Effort now names the
TypeScript SPA and carries ADR-006's one-day-ceiling risk note. **CLOSED.**

### DDR-3 — ADR-005 does not fix the replay status code

**Where**: `adr-005-idempotency-replay.md` § Decision — "Same key, same
fingerprint | Re-render from transaction_id, return the original result". New
keys are explicitly 201; a replay is unspecified.

**Ruling** (user, 2026-08-18): a replay answers **200**, with a body identical
to the original. 201 means created and a replay creates nothing; the status also
makes the `replayed` log field assertable end to end.

**Upstream fix applied 2026-08-18**, confirmed by the DESIGN reviewer:
`adr-005-idempotency-replay.md` § Decision now carries a Status column —
201 new, **200 replay**, 409 conflict, 400 missing. **CLOSED.**

---

## Findings raised here, since closed

### U-1 — slice 03 says "killing the process between them is not possible", which is not a testable criterion

**Where**: `slices/slice-03-idempotent-retry.md` § Acceptance criteria:

> - [ ] Key and transaction commit atomically — killing the process between them
>   is not possible because they are one write

**Problem**: this states a *reason*, not an observation. There is no moment to
kill and therefore no test to write — the criterion asserts the absence of a
window rather than any outcome a scenario could observe. Written as-is it will
be silently skipped, or worse, ticked off without a test behind it.

**What DISTILL did instead**: the atomicity guarantee is covered observably by
`milestone-03` "A refused transfer does not consume its key" and by
`milestone-01` "A transfer interrupted halfway leaves no half-applied movement",
which assert what a caller can actually see after an interruption. The
one-write claim is a design property enforced by DDD-8, verified by code review
rather than by a scenario.

**Resolved 2026-08-18.** The DISCUSS reviewer confirmed the finding and
directed the rewording; `slices/slice-03-idempotent-retry.md` now reads "After
any interruption, a key exists if and only if its transaction exists —
observable by retrying with the same key", with the original phrasing and the
reason for the change recorded inline. **CLOSED.**

### U-2 — KPI-1 asserts on a healthy ledger, but nothing gates the corrupted case's `elapsed_ms`

**Where**: `docs/product/kpi-contracts.yaml`, KPI-1 guardrail — `elapsed_ms`
warns above 2000ms, on the `ci` and `populated` environments.

**Problem**: the guardrail exists to signal when D9's deferred checkpointing
needs revisiting. But the full scan's cost is a function of total history, and
the `corrupted` environment is reached *from* `populated` — so the scan that
matters most for degradation (the one that has to attribute drift across every
account) is not in the guardrail's environment list. This is a small gap and it
gets larger as history accumulates.

**Resolved 2026-08-18.** The DEVOPS reviewer confirmed the finding, rated it
high, and made it the condition of its approval. `kpi-contracts.yaml` KPI-1 now
reads `environment: [ci, populated, corrupted]`, with the reasoning recorded in
a `note`. DISTILL made the edit only once the owning wave's gate had directed
it. **CLOSED.**

---

## Non-findings, recorded so they are not re-investigated

- **Slice-03's "store the response body" recommendation** is already superseded
  by DDD-8 with a Changed Assumptions entry in the slice brief itself. Not a
  live contradiction.
- **`rigor.mutation_enabled: false` versus nightly-delta mutation testing** is
  already reconciled in `feature-delta.md` § DEVOPS / Mutation testing strategy.
  The two settings describe different surfaces. No action.


---

## Review-cycle findings (R-1 added 2026-08-18, R-2 added 2026-08-19)

### R-1 — DISTILL omitted the contract-shape mandate on first pass

**Raised by**: `@nw-acceptance-designer-reviewer`, which rejected the wave.

**What was missed**: designer principle 14 (2026-05-15, identity-essential)
requires every Gherkin scenario to carry a
`@contract-shape:<pure-function | bounded-change | unbounded-preservation>` tag.
All 53 scenario blocks were authored untagged.

**Why it was missed, honestly**: the mandate lives in the acceptance-designer
*agent definition*, not in the `nw-distill` skill, and this wave was executed
from the skill. That is an explanation, not an excuse — the tag is machine-
parseable precisely so its absence is caught mechanically, and it was.

**Fixed**: all 53 blocks tagged, classified by the shape of each scenario's
`When`. DESIGN gained a matching § Contract shape per component so the
per-scenario tags have an authoritative source to agree with, rather than being
53 independent judgements.

**Worth propagating**: the `nw-distill` skill's self-review checklist has 15
items and none of them is the contract-shape tag. Adding it there would have
caught this before review. That is a framework fix, not a project one.

### R-2 — no HTTP status code is asserted anywhere in the suite

**Status: CLOSED 2026-08-21, DELIVER step 05-04.** `Answer` gained a
`Status int` field; `decodeAnswer` now captures `response.StatusCode`
instead of discarding it; `ledger_assertions.go`'s `statusFor(RefusalKind)`
transcribes ADR-008's status table verbatim across all nine sealed members
and is wired into the refusal/accept/replay assertions exactly as the
reviewer's recommended fix shape below describes — folded into existing
assertions, zero `.feature` changes, zero new step decorators. DDR-3 (replay
answers 200) is now asserted. Verified: all 49 acceptance scenarios stayed
green when the assertion was added, meaning production had been built
correctly against ADR-008 throughout DELIVER — the gap was in test coverage
only, not in behavior.

**Raised by**: `@nw-acceptance-designer-reviewer` (CONDITIONALLY_APPROVED, one
HIGH condition) against the three scenarios added when C2b and C6a closed.
Verified suite-wide and **pre-existing** — not introduced by that pass.

**Where**: `tests/acceptance/ledgercore/`. The `Answer` struct in
`domain_types.go` has no status field; `ledger_assertions.go` makes zero
assertions on status; none of the seven `.feature` files mentions a status code.
The only `Status` in the suite is a JSON *body* field in `decodeAnswer`.
`decodeAnswer` reads `response.StatusCode` solely to classify the outcome as
`Refused` when it is `>= 400`, then discards it.

**Why it matters**:

- **DDD-17's status-follows-site rule has nothing testing it.** 400
  not-a-command · 401 not-identified · 404 names-something-absent · 409
  identifier-already-bound · 422 rules-refuse-it. Production could answer 422
  for `malformed_request` and the suite stays green, because only the `error`
  field in the body is checked.
- **DDR-3 is untested.** The user personally ruled that a replay answers **200**
  rather than 201, and ADR-005 records the reason as "the status makes the
  `replayed` log field assertable end to end". Nothing asserts it. The
  distinction the user chose is currently invisible to the suite.
- The gap is not specific to the new scenarios. It covers `POST /accounts` 201,
  replay 200, key conflict 409, insufficient funds 422 and the two 401 auth
  scenarios equally. The C2b and C6a additions are simply the first whose
  *entire point* is a status distinction — `malformed_request` 400 versus
  `invalid_amount` 422 differ in nothing else observable — which is why it
  surfaced there.

**Reviewer's recommended fix**: add a `Status int` field to `Answer`, capture
`response.StatusCode` in `decodeAnswer`, and assert it. Of three shapes offered
the reviewer preferred folding status into the existing refusal assertions
rather than adding a per-scenario status line, on the grounds that a status line
in 62 scenarios is noise.

**DISTILL's view on the shape, recorded alongside it**: agreed on folding, and
for a stronger reason than noise. A status code in Gherkin is technical jargon
and would violate business-language purity (Pillar 1) in every scenario it
touched — the operator does not think in 422. But the fold should go one step
further than the refusal path, because status is not only a refusal concern:
201 create and 200 replay are the DDR-3 half of the gap. The shape to
implement is

- a `statusFor(RefusalKind) int` table in `domain_types.go`, transcribed from
  ADR-008's status table and covering the sealed set — so a tenth member cannot
  be added without deciding its status, and the `exhaustive` linter can cover
  the switch exactly as it covers the wire mapping;
- `ThenItIsRefusedAs` asserting `Answer.Status == statusFor(kind)` in addition
  to the `error` field it already checks;
- `ThenTheTransferIsAccepted` / `ThenTheAccountIsCreated` asserting 201 and
  `ThenTheRepeatIsAnsweredAsAReplay` asserting 200, which closes DDR-3.

Net cost: one struct field, one line in `decodeAnswer`, one table, four
assertion lines. Zero new step decorators and zero `.feature` changes — the
status becomes a property of the taxonomy rather than of each scenario, which is
where it belongs, since ADR-008 decides it once per member and not once per
scenario.

**Status: OPEN — deferred by user decision, not dropped.** The user chose to
move to DELIVER rather than run another authoring round, with the condition
recorded here instead. Owned by DISTILL. **DELIVER should treat ADR-008's status
table as the authority when implementing the HTTP adapter's status mapping, and
should not expect the acceptance suite to catch a wrong status** — it will not.
Closing this is the natural companion to DELIVER's existing obligation to add
the `exhaustive` linter over the adapter's wire-mapping switch: both exist so
the status table stays honest, and neither is discharged today.
