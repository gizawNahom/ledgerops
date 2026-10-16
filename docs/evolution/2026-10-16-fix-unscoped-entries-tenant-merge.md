# Evolution — fix-unscoped-entries-tenant-merge

**Status: DELIVERED (bugfix).** 3 roadmap steps complete (`des-verify-integrity`:
All 3 steps have complete DES traces). Review: first verdict NEEDS_REVISION, one
revision pass (`301c80a`), then accepted. Commits `f2bf4e0..301c80a` on main.
RCA: `docs/feature/fix-unscoped-entries-tenant-merge/rca.md`.

---

## Defect and symptom

The operator's unscoped `GET /accounts/{id}/entries` merged every tenant's
entries for a shared account id. Entries carried no tenant attribution, and
`running_balance` was folded across tenants. The console drift drill-down
therefore showed the merged history. This closes follow-up (a) of
`2026-10-16-fix-trial-balance-tenant-collision.md`; follow-up (d), the pact
provider fake, is partly addressed (see follow-ups).

## Fix and commits

- `ports.TenantEntry` on the `EntriesFor` port. `domain.Entry` is untouched
  (ADR-007).
- Postgres selects `tenant_id`; the unscoped read orders by
  `tenant_id, recorded_at, sequence`.
- `TracedEntry.TenantID` added; `runningBalances` resets per tenant.
- `tenant_id` is emitted on the wire for every caller.
- `entriesScopeFromQuery`: the operator may narrow with `?tenant_id=`; a tenant
  key naming itself is treated as naming none; a tenant key naming any other
  tenant is refused with `account_not_found`, naming the requested account.
- Console: the drift row carries `tenantId`; a Tenant column; rows keyed by
  tenant+account; the drill-down sends `?tenant_id=`; `EntryTrace` shows the
  tenant. The consumer pact was regenerated and the provider verifies it.

| Commit | Roadmap step |
|---|---|
| `f2bf4e0` | 01-01: port and use case |
| `2ebf9d0` | 01-02: HTTP |
| `01ccd82` | 01-03: console |
| `16e0f00` | Refactor: stale comments after tenant-tagged entries, tidy `runningBalances` |
| `301c80a` | Review revision (D1, D5) |

## Decisions (2026-10-16, rca.md §7)

1. The bare operator read returns every tenant's entries, tagged with
   `tenant_id` and grouped by tenant, with the running balance restarting per
   tenant. It does not return 409.
2. `tenant_id` is emitted for all callers. A tenant key naming a different
   tenant is refused.

The user approved the review triage (fix D1 and D5; D3 and D4 do not apply) and
approved rewording commit `775c16e`'s co-author trailer via a rebase. It became
`2ebf9d0`, and the downstream hashes changed.

KPI-M4 amendment (approved 2026-10-16): unscoped OperatorKey
`GET /accounts/{id}/entries` rows gain one additive field, `tenant_id`, and are
grouped by tenant with a per-tenant `running_balance`. On a single-tenant ledger
rows read `"tenant_id":"tnt_legacy_seed"`; the response otherwise keeps its
fields and status. An optional, operator-only `?tenant_id=` narrows the read.
Reason: an operator must be able to tell whose entries they are reading. The
target text is unchanged.

## Tests

- **Acceptance**: 9 scenarios (11 runs) in
  `tests/acceptance/multitenancy/milestone-03c-operator-entries-with-shared-account-names.feature`,
  all tagged `@contract-shape`.
- **Adapter**: `TestTransactionRepository_EntriesFor_UnscopedAttributesEachEntryToItsTenantScopedSeesOnlyItsOwn`
  replaces the old merge pin.
- **Unit** (`internal/app/usecases_test.go`): three tests, including the
  per-tenant running-balance property.
- **vitest**: 52/52.
- Final acceptance runs: multitenancy 32/32, ledgercore 93/93,
  intertenanttransfer 41/41.

### RED evidence and process notes

- Step 01-01: RED was shown after implementation, by neutralising the
  per-tenant reset. Its COMMIT phase was logged by the user, after the
  auto-mode classifier denied the crafter's `des-log-phase` call.
- Step 01-02 (review D2): RED, GREEN and COMMIT were batch-logged with one
  timestamp. The orchestrator independently reproduced RED by restoring the
  pre-fix `handlers.go` in a scratch worktree: all 03c scenarios failed on
  assertions (entries carried no tenant, refusals returned 200). The execution
  log was not edited.

## Review

First verdict NEEDS_REVISION; one revision pass (`301c80a`).

- **D1 (fixed):** the refusal body echoed the tenant id, or whatever was sent in
  `?tenant_id=`, into `account_id`. It now names the requested account. The
  acceptance step was tightened and a scenario added for narrowing to a
  never-provisioned tenant.
- **D5 (fixed):** stale comments.
- **D3 (not applicable):** CONTRACT_SHAPE docstrings and Outcome anchors are not
  a convention in this repo (0 occurrences).
- **D4 (not applicable):** before/after snapshots on refusals of a read-only
  GET; none of the repo's unbounded-preservation scenarios take them.
- **D2:** see process notes above.

## Rollout

Deploy the server before the console. The console sends `?tenant_id=` and reads
`tenant_id`; an old server ignores the former and omits the latter.

## Open follow-ups (out of scope)

From this review (non-blocking):

- `runningBalances` relies on adapter grouping; a keyed fold would be
  order-independent.
- No unit test for the three branches of `entriesScopeFromQuery`.
- `apiClient.ts` does not URL-encode `accountId` in the path.
- `EntryTrace`'s row key (counterparty+recordedAt) can collide.
- The app fake's single-tenant `entriesWithTenantIn` branch ignores scope.

Carried forward from the trial-balance evolution doc:

- **(b)** `driftedAccounts` iterates stored accounts only; relies on migration
  0003's composite foreign keys.
- **(c)** Mixed currencies in `TrialBalance` and `ComputedBalances` overwrite
  each other per (tenant, account). Latent; USD only.
- **(d)** Pact provider fake (`internal/adapters/http/pact_provider_test.go`),
  partly addressed: it now honours scope and has a `tnt_acme` provider state,
  but its accounts map is still keyed by bare id.
- **(e)** Test-support corruption hook `targetEntry` (`postgres/store.go`) is
  not tenant-qualified.

(a) is closed by this feature.

## Permanent artifacts

- `docs/product/kpi-contracts.yaml` KPI-M4 amendment (second entry).
- `docs/product/architecture/brief.md`: dated amendment after the 2026-10-16
  trial-balance amendment, and a dated note on the dual-mode port table row for
  `GET /accounts/{id}/entries`.
- `docs/feature/fix-unscoped-entries-tenant-merge/` retained as the DES audit
  record (`rca.md`, `deliver/roadmap.json`, `deliver/execution-log.json`).
