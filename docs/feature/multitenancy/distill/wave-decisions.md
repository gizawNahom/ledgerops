# Wave Decisions Summary — DISTILL (`multitenancy`)

Full DISTILL narrative lives in `docs/feature/multitenancy/feature-delta.md`
§§ Wave: DISTILL (this project's single-narrative convention, per the
`ledger-core` precedent) — this file is the same kind of mandatory
cross-reference summary the DESIGN and DEVOPS waves already produced for this
feature (`docs/feature/multitenancy/design/wave-decisions.md`,
`docs/feature/multitenancy/devops/wave-decisions.md`), continued here for
consistency even though the generic `nw-distill` skill's default is a single
narrative file with no separate `distill/wave-decisions.md`. Deviating from
the skill's stated default to match this project's own established
convention is a deliberate `--policy=inherit`-style choice, not an oversight.

## Wave-Decision Reconciliation HARD GATE

Read `discuss/`, `design/`, `devops/` wave-decisions.md in full (this feature
keeps them under `docs/feature/multitenancy/{discuss,design,devops}/` per its
own established layout — no separate `discuss/wave-decisions.md` file exists
because DISCUSS's narrative lives entirely in `feature-delta.md`; DESIGN's and
DEVOPS's own summary files were read directly). Checked every DISCUSS decision
against DESIGN and DEVOPS for contradiction (email-notifications-vs-in-app
style checks): **zero contradictions found.** DESIGN's three legs (system,
domain, application) explicitly confirm zero new system-level footprint and
resolve every DISCUSS-flagged open question (DDD-18 rescoping → I9, I8
enforcement mechanism → construction-time, credential mechanism → DDD-22,
cross-tenant refusal shape → reuse `account_not_found`, unscoped-verdict
semantics → platform-wide aggregate). DEVOPS's own contradiction check
(`devops/wave-decisions.md` § Contradiction Check) already confirmed the same
against DESIGN. **Reconciliation passed — 0 contradictions.**

## Language + policy + port bootstrap

- `[lang-mode] go` — detected from `go.mod` (module `ledgerops`, Go 1.25).
- `[policy-mode] inherit` — `docs/architecture/atdd-infrastructure-policy.md`
  already existed (bootstrapped 2026-08-18 during `ledger-core` DISTILL).
  Six new/extended rows appended (§ Driving, one new row for `TenantRepository`
  under § Driven internal); no row rewritten from scratch, no `--policy=fresh`.
- `[port-mode] inherited` — `tests/common/statedelta/state_delta.go` already
  present (same 2026-08-18 bootstrap). Not re-bootstrapped.

## Scenario authoring

- **18 scenarios total**: 1 walking skeleton (chains slice 01→02, per the
  DISCUSS-locked "this feature's own — slice 01 alone is not sufficient"
  decision) + 5 slice-01 + 7 slice-02 + 5 slice-03. Every scenario lifted and
  adapted from DISCUSS's own UAT Gherkin (`feature-delta.md` § Wave: DISCUSS
  / User stories with elevator pitches) — DISCUSS's `tk_acme`/`tk_beacon`
  shorthand was rewritten into consistent domain-language tenant-name
  phrasing (Pillar 1); no scenario invents a business rule DISCUSS/DESIGN did
  not already settle.
- **Error/edge-path ratio**: 8 of 18 (44%) are refusal/isolation-proof
  scenarios — exceeds the 40% floor.
- **Contract-shape tags** (2026-05-15 mandate): every scenario carries exactly
  one `@contract-shape:` tag — `bounded-change` for writes, `unbounded-preservation`
  for refusal/isolation-proof scenarios, `pure-function` for read-only
  verdict/entries checks.
- **Tier B (state-machine PBT): not added.** This feature's three journeys
  (provision, operate, verify) are each 1-2 scenario-deep from any one
  starting Given, and the input space (tenant/account names, amounts) is not
  domain-rich enough to clear Mandate 10's bar — config-shaped in the sense
  that section describes (a handful of named entities, not free-text/payload
  variety). Tier A alone is the correct call, not a shortcut.

## Mandate 8 (Universe-bound assertion) — honoured directly, not via the literal helper

Every mutating Then-assertion in `world.go` compares exactly one declared,
port-exposed observable against its expected value (never an internal
struct field) and states its own mismatch reason — the same choice
`tests/acceptance/ledgercore/ledger_assertions.go` already made and documents
in its own OPS-5 section: `statedelta.AssertStateDelta` needs a `testing.TB`
to call `Fatalf` on, and this suite's godog runner drives scenarios from
`TestMain`, not a `*testing.T` subtest. Continuing that exact precedent here
rather than inventing a second convention for one feature.

## Mandate 7 (RED-ready scaffolding) — minimal footprint, verified live

Only one production scaffold was needed: `POST /tenants` →
`scaffold("provision_tenant")` in `internal/adapters/http/router.go`, mounted
behind the existing `requireOperatorKey` group (DDD-22 reuses that middleware
verbatim, so the non-admin/unissued-credential refusal scenarios pass without
any new code). No domain- or application-layer scaffold was needed: this
suite's step definitions invoke driving ports exclusively (Mandate 1) and
never import `internal/domain` or `internal/app` directly, so the only import
surface DISTILL owes a scaffold to is the HTTP route. Reused ledger-core's own
established `scaffold()` helper — a real 501 HTTP response carrying
`__SCAFFOLD__` in the body — rather than inventing a second RED convention.

**Verified live, not asserted**: `go build ./...`, `go vet ./...`, and
`go test -c` all pass with zero errors. The full suite was run against this
environment's real Docker/Testcontainers (`LEDGEROPS_AT_TAGS=""`, all 18
scenarios including `@pending`): **18 scenarios, 8 passed, 10 failed, every
failure `MISSING_FUNCTIONALITY`.** Full breakdown:
`docs/feature/multitenancy/distill/red-classification.md`. Two real
implementation bugs were caught and fixed during this run (not merely
theorized): (1) `authenticate()` panicked when a tenant's credential was not
yet resolvable — panicking aborts the whole godog process rather than failing
one scenario, which is a BROKEN-shaped failure mode Mandate 7 exists to
prevent; fixed to present a synthetic, certainly-wrong bearer instead, so the
call fails at the Then assertion like every other RED. (2) driving-port call
methods that could run before any Given started the store
(`nil` `httptest.Server`) now call `EnsureStarted` defensively. Both fixes are
in the acceptance-test infrastructure only — no production code was touched
beyond the one scaffold route.

## Outcomes registry (per DISCUSS#D-5 grain)

`nwave-ai outcomes register` is broken in this install (no packaged
`schema.json` — the same pre-existing finding `registry.yaml`'s own header
comment and DESIGN's `wave-decisions.md` § Cross-cutting confirmations already
recorded). Written directly in the `outcome_to_dict` shape, per that same
precedent: **OUT-11** (`ProvisionTenant`, operation), **OUT-12** (I8,
invariant), **OUT-13** (I9, invariant), **OUT-14** (I10, invariant) appended;
**OUT-1, OUT-2, OUT-4, OUT-5** extended in place (widened `inputs`/`output`
shape only — `kind` and `artifact` unchanged), exactly as DESIGN's own Outcome
Collision Check flagged for this wave to do. Zero collisions.

## Mandate-12 (SSOT + zero duplication)

- **Criterion 1** (domain types module): `tests/acceptance/multitenancy/domain_types.go`
  — every domain noun (`TenantName`, `Caller`, `RefusalKind`, `TenantAnswer`,
  `BooksReport`, `DriftRow`) is a typed Go declaration. `Money`/`AccountKind`
  are deliberately re-exported from `tests/acceptance/ledgercore` rather than
  redeclared — this project's existing SSOT for those two nouns, per the
  header comment's own stated line for where reuse beats a fresh type
  (real parsing logic worth not duplicating vs. a plain labelled string).
- **Criterion 2** (typed composition parameters): every `World` method takes
  `TenantName`/`AccountName`/`Caller`/`Money`/`AccountKind`, never a raw
  `string` where a domain type exists.
- **Criterion 3** (no business logic in step bodies): every step in
  `steps_multitenancy_test.go` is a regex capture into a domain-typed
  parameter followed by exactly one `w.<Method>(...)` call — no
  `if`/`for`/`while` in a step body (spot-checked; a mechanical AST scan is a
  DELIVER-time CI concern, not run here).
- **Criterion 4** (step-reuse-ratio, informational): measured mechanically
  (`grep -c` per the skill's own formula) — 102 Given/When/Then/And/But
  occurrences across the four `.feature` files over 41 unique `ctx.Given/
  When/Then(...)` registrations in `steps_multitenancy_test.go` ≈ **2.49×**.
  Above the F-ENTERPRISE 1.43× post-refactor ceiling discovered elsewhere in
  nWave's own dogfooding, below a 4× target — expected for a 3-story feature
  whose scenarios are largely narrative variations (who's calling, which
  tenant, which account) rather than a large enum-typed parameter space. Not
  a redesign signal; recorded per the mandate's own informational-only
  discipline.

## Test placement

`tests/acceptance/multitenancy/` — a sibling package to
`tests/acceptance/ledgercore/`, not an extension of it. Justified in
`world.go`'s own header comment: this feature needs its own composition root
(concurrent multi-tenant identities against one server) and its own walking
skeleton (DISCUSS's own locked decision), which `ledgercore`'s `Ledger` struct
was never shaped for. Precedent: `ledger-core-console`'s own `web/console/`
is likewise a sibling test tree, not a subdirectory of `ledgercore`'s.

## Judgment calls made this wave, flagged for review

- DISCUSS's UAT Gherkin used `tk_acme`/`tk_beacon` shorthand; DISTILL rewrote
  every scenario into consistent tenant-name phrasing. No business rule
  changed — a Pillar 1 (domain language) refinement, not a scope decision.
- Console-compat scenarios (`@console-compat`) assert "unchanged in shape"
  informally (status + presence of expected fields), not a byte-for-byte
  diff against a recorded golden response. A literal byte-diff would need a
  captured pre-multitenancy response fixture this feature does not have
  (ledger-core's own suite predates any such capture). Flagged for DELIVER:
  if `console-compat`'s own CI job (DEVOPS OPS-12) needs a stricter diff,
  that is an assertion-tightening task at GREEN time, not a DISTILL scope gap
  — the scenario's intent (the call must keep succeeding, unscoped) is
  already correctly captured.
- `World`'s `Clock`/`IDGenerator` are NOT faked (unlike `ledgercore`'s
  `Ledger`) — no scenario in this feature's scope needs a pinned value.
  Documented as a deliberate scope-driven choice in `world.go`'s header, not
  a policy deviation (the Architecture of Reference classifies these as
  driven external/non-deterministic ports eligible for a fake, not mandating
  one — Ledger-core's own suite fakes them for a different reason, exact
  transaction-id/timestamp assertions this feature's scenarios never make).

## Pre-DELIVER gate

**PASS.** `docs/feature/multitenancy/distill/red-classification.md` — 18
scenarios, 8 passed, 10 failed, zero non-`MISSING_FUNCTIONALITY` failures.

## Handoff

To `nw-software-crafter` (DELIVER wave), pending the Final Wave Review Gate
(four parallel reviewers against the full `feature-delta.md` chain).
Deliverables: this section + `feature-delta.md` §§ Wave: DISTILL +
`tests/acceptance/multitenancy/*` + the one scaffold route in
`internal/adapters/http/router.go` + the appended
`atdd-infrastructure-policy.md` rows + the extended `outcomes/registry.yaml`.
