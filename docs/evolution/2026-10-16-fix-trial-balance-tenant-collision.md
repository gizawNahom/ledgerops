# Evolution — fix-trial-balance-tenant-collision

**Status: DELIVERED (bugfix).** 2 roadmap steps complete (`des-verify-integrity`:
2 of 2 steps have complete DES traces). Review: APPROVED. Commits
`7fc6468..91676a9` on main. RCA: `docs/feature/fix-trial-balance-tenant-collision/rca.md`.

---

## Defect and symptom

With tenants A and B each holding a correctly funded `treasury`, the
platform-wide (unscoped) `GET /health/trial-balance` answered "Books balance:
NO" with `treasury` stored -100.00 and computed -200.00. Nothing was wrong
with the books. The scoped `?tenant_id=` path was correct throughout.

## Root cause (two halves, both required)

Migration 0003 changed `accounts` identity to `(tenant_id, id)`, but the
unscoped verification path was never given a tenant-qualified account identity.

- **A, computed side merged by account id.** `ComputedBalances` on the
  unscoped path grouped by bare `account_id` and returned `map[string]Money`,
  so two tenants' `treasury` entries became one -200 row.
- **B, stored side pinned to the legacy tenant.** `VerifyBooks` hard-wired the
  accounts leg to `tnt_legacy_seed` when unscoped, so the stored side was one
  tenant's accounts while the computed side was every tenant's entries. This
  half also hid drift: with no legacy account involved, the stored side was
  empty and the check answered YES (false negative), so drift in any
  non-legacy tenant was never detected. For a books-integrity control this is
  the more dangerous half.

Fixing either half alone is insufficient (B alone: two drift rows of -100 vs
merged -200; A alone: non-legacy tenants never checked).

Process cause: `TestVerifyBooks_UnscopedCallStaysPlatformWide` pinned the
legacy-only enumeration as "byte-identical to pre-multitenancy", and no
fixture exercised colliding account ids on the unscoped path.

## Fix and commits

Both legs compare per `(tenant_id, account_id)`; the `legacyTenantID` special
case is gone; `Drift` and `driftWire` carry `tenant_id`.

| Commit | Change |
|---|---|
| `7fc6468` | Step 01-01. Tenant-qualified comparison (ports, postgres accounts/transactions, `VerifyBooks`, `driftedAccounts`); regression tests; fakes and pact provider fake updated to new port shapes |
| `8bf9578` | Unit asserts on `Drift.TenantID` |
| `c8b9fb7` | Step 01-02. `driftWire` gains `tenant_id`; acceptance scenarios un-skipped |
| `eb05f78` | Refactor: comments the tenant-qualified verdict made stale |
| `a8dc8ed` | Comment fixes (VerifyBooks rewrap, `BooksReport` doc names the tenant) |
| `91676a9` | Adapter test tightened to assert per-tenant attribution of unscoped computed balances |

## Tests added and RED evidence

- **Unit** (`internal/app/usecases_test.go`): property tests for shared
  account ids across tenants not being drift, and for exactly the one drifted
  (tenant, account) being reported;
  `TestVerifyBooks_UnscopedCallChecksEveryTenantNotJustTheLegacyOne` replaces
  the legacy-only pin.
- **Adapter** (`internal/adapters/postgres/transactions_platform_wide_test.go`):
  `ComputedBalances(Unscoped())` keeps two tenants sharing an account id apart.
- **Acceptance** (`tests/acceptance/multitenancy/milestone-03b-platform-wide-books-with-shared-account-names.feature`,
  3 scenarios, contract-shape tagged): legacy plus one tenant each funded
  `treasury` balance; drift in a tenant's `treasury` attributed to that tenant,
  not its legacy namesake; drift in one of two ordinary tenants found and
  attributed to it alone.
- **RED evidence**: before the fix the tests reproduced the reported
  -100 stored / -200 computed false drift for colliding ids, and a false
  "Books balance: YES" for drift in non-legacy tenants.

## Decisions

From rca.md §7 (2026-10-16):
- Root cause and fix direction approved: tenant-qualified keys on the
  platform-wide path, remove the `legacyTenantID` pin, add `TenantID` to
  `Drift`/`driftWire`.
- `EntriesFor` unscoped is OUT OF SCOPE; tracked as follow-up (a).
- Single-tenant behaviour must be unchanged.

KPI-M4 amendment (approved 2026-10-16): unscoped `GET /health/trial-balance`
and `GET /console/verdict` drift rows gain one additive field, `tenant_id`.
On a single-tenant ledger with drift, rows read `"tenant_id":"tnt_legacy_seed"`;
the no-drift response stays byte-identical. Reason: an operator must be able
to tell which tenant's same-named account drifted. Recorded as an `amendments`
entry on KPI-M4 in `docs/product/kpi-contracts.yaml`; the target text is
unchanged.

## Review

Adversarial review: APPROVED. Follow-up commits `eb05f78`, `a8dc8ed`, `91676a9`
address review observations (comments, tighter adapter assertion).

## Consequence to expect

The first platform-wide check on multi-tenant production data may report
drift that was previously invisible (non-legacy tenants). That is correct
behaviour; `SetDriftedAccounts` may rise for the right reasons.

## Open follow-ups (out of scope)

No dedicated follow-up register exists in this repo (precedent: the
inter-tenant-transfer evolution doc records them inline), so they are
recorded here.

- **(a) Unscoped `GET /accounts/{id}/entries`**
  (`postgres/transactions.go` `EntriesFor`, ~line 108, `WHERE account_id = $1`)
  merges the entries of every tenant sharing the id. Intended behaviour is a
  product decision.
- **(b) `driftedAccounts` iterates stored accounts only.** A computed entry
  with no matching stored account would never be flagged. Impossible today
  because of migration 0003's composite foreign keys, but the integrity check
  relies on them.
- **(c) Mixed currencies** in `TrialBalance` and `ComputedBalances`: multiple
  currencies per (tenant, account) overwrite each other. Latent; the ledger
  is USD only.
- **(d) Pact provider fake** (`internal/adapters/http/pact_provider_test.go`)
  models a single tenant and ignores scope. Add a comment, or a collision
  provider state, if console contract coverage ever needs multi-tenant.
- **(e) Test-support corruption hook `targetEntry`** (`postgres/store.go`) is
  not tenant-qualified.

## Permanent artifacts

- `docs/product/kpi-contracts.yaml` KPI-M4 amendment.
- `docs/product/architecture/brief.md`: dated amendment after the
  "Mandatory CI-gated byte-identical scenarios" paragraph.
- `docs/feature/fix-trial-balance-tenant-collision/` retained as the DES audit
  record (`rca.md`, `deliver/roadmap.json`, `deliver/execution-log.json`).
