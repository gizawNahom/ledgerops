# RCA: platform-wide GET /health/trial-balance reports false drift when two tenants share an account id

Method: static analysis only. Not reproduced live (a live repro needs a new test). The reported figures (stored -100, computed -200) match the mechanism below exactly.

## 1. Root cause chain

PROBLEM: with tenants A and B each holding a correctly funded `treasury`, the unscoped check says "Books balance: NO" with treasury stored -100.00 and computed -200.00.

**WHY 1 (symptom).** `driftedAccounts` compares one stored `treasury` balance (-100) with a computed balance (-200) and finds a mismatch.
- Evidence: `internal/app/usecases.go:695-724`. It looks up `computed[account.ID()]` at line 700 and reports a Drift when `balance != stored` at line 709.
- -200 is the sum of both tenants' treasury entries. -100 is the stored balance of one tenant's treasury.

**WHY 2 (the computed side is merged across tenants).** On the unscoped path, `ComputedBalances` groups entries by account id alone and returns `map[string]domain.Money` keyed by bare `account_id`.
- Evidence: `internal/adapters/postgres/transactions.go:186-187`: `SELECT account_id, currency, SUM(amount_minor) FROM entries GROUP BY account_id, currency`.
- Line 205 (`balances[accountID] = balance`) is the last write for that id. For a given currency the SQL has already summed both tenants, so the two treasuries become one -200 row.

**WHY 3 (the stored side sees only one tenant).** `VerifyBooks` hard-wires the accounts leg to `legacyTenantID` (`tnt_legacy_seed`) when the scope is `Unscoped()`.
- Evidence: `usecases.go:636` (`accountsTenantID := legacyTenantID`) and line 648 (`Accounts().All(ctx, accountsTenantID)`). The SQL is `WHERE tenant_id = $1` at `internal/adapters/postgres/accounts.go:138-139`.
- So the stored side is one tenant's accounts (the -100) and the computed side is every tenant's entries. The two legs of the comparison cover different populations.
- The reported figures only arise if the legacy tenant holds one of the two treasuries.
- If neither tenant were legacy, `accounts` would be empty and the check would wrongly say YES. That is the false-negative twin of this bug.

**WHY 4 (design allowance).** Migration 0003 deliberately changed the key from `id` to `(tenant_id, id)` (`internal/adapters/postgres/migrations/0003_tenants.up.sql:4-7, 74`). The port and use case were then only partly migrated.
- Step 03-01 kept the unscoped path "byte-identical to the pre-multitenancy contract" (`usecases.go:34-44, 607-612`). The claim was that unscoped = legacy accounts plus platform-wide sums. That only holds while all rows belong to a single tenant.
- Evidence: the `ComputedBalances` return type is `map[string]domain.Money` (`ports.go:152`) and `Accounts.All` takes a single `tenantID` (`ports.go:122`). Neither port can express a (tenant, account) pair on the platform-wide path.

**WHY 5 (root cause).** The platform-wide verification path was never given a tenant-qualified identity for accounts.
- The "byte-identical to legacy" regression pin, `TestVerifyBooks_UnscopedCallStaysPlatformWide` (`usecases_test.go:439-456`), locks in the legacy-only enumeration (it asserts `lastAccountsAllTenantID == tnt_legacy_seed`).
- No test or acceptance scenario exercises the unscoped check with two tenants sharing an account id. `milestone-03-verify-one-tenants-books.feature` lines 44 and 51 only assert that the unscoped call "keeps succeeding".
- ROOT CAUSE A (design): the unscoped read model keys balances by `account_id` alone.
- ROOT CAUSE B (design): the unscoped accounts leg is hard-pinned to the legacy tenant.
- ROOT CAUSE C (process): the regression pin enshrined the old behaviour, and no multi-tenant fixture with colliding ids covers the unscoped path.

Backward validation:
- A and B together produce one stored treasury (-100) against a merged computed treasury (-200), so Drift appears.
- Fixing B alone (enumerate all tenants) still yields two drift rows (-100 stored vs merged -200 each).
- Fixing A alone never checks non-legacy tenants.
- Both must be fixed; they do not contradict each other.

## 2. Contributing factors and other code with the same flaw

| Location | Flaw | Impact |
|---|---|---|
| `postgres/transactions.go:108-109` (`EntriesFor`, unscoped) | `WHERE account_id = $1` with no tenant | Operator reads of `/accounts/{id}/entries` merge entries of every tenant with that id. Same class of bug. |
| `postgres/accounts.go:121` (`ExistsAnyTenant`) | Deliberately tenant-agnostic | Only a courtesy existence check. Harmless alone. |
| `postgres/store.go:289` (`targetEntry`, test/ops hook) | `WHERE account_id = $1` | Corruption demo can hit the wrong tenant's entry. Test-support only. |
| `usecases.go:636-648` | Unscoped enumerates legacy only | Drift in any other tenant is never detected on the platform-wide check (false negative). The more dangerous failure for a books-integrity control. |
| `postgres/transactions.go:123-169` (`TrialBalance`, unscoped) | Sums all tenants; keeps first row's currency and ignores others (lines 151-155) | Correct for single currency (`ledgerCurrency` is USD, `usecases.go:28`). Latent trap if currencies are added. |
| `Drift` (`usecases.go:764`) and `driftWire` (`handlers.go:812`) | Carry `AccountID` only | Operator cannot tell which tenant's `treasury` drifted. |
| `pactFakeAccountRepository.All` (`pact_provider_test.go:419`), `fakes_test.go:213, 289` | Fakes mirror the single-tenant, id-keyed shape | Would hide a regression unless updated. |

`TransferCoordinator` (`transfer_coordinator.go:540`) calls `All(ctx, tenantID)` per tenant, so it is fine.

## 3. Proposed fix

Make the unscoped path compare per (tenant_id, account_id).
1. Port, `ports.go:122` and `:152`. Add a tenant-qualified key:
   ```go
   type AccountKey struct{ TenantID, AccountID string }
   ComputedBalances(ctx, scope) (map[AccountKey]domain.Money, error)
   ```
   Preferred: change `Accounts().All` to take a `TenantScope`. Unscoped drops the `tenant_id` filter and returns `tenant_id` with each account (or a `TenantAccount{TenantID, Account}` wrapper type).
2. SQL, `transactions.go:184-187`. Select `tenant_id` in both branches: `SELECT tenant_id, account_id, currency, SUM(amount_minor) ... GROUP BY tenant_id, account_id, currency`. Key the map by `AccountKey{tenant, account}`.
3. `accounts.go:137`. Add an unscoped branch: `SELECT tenant_id, id, kind, balance_minor, currency FROM accounts ORDER BY tenant_id, id`. Extend `scanAccount` to take the tenant per row.
4. `usecases.go:631-679`. Delete the `legacyTenantID` special case. Pass `scope` through to both reads. Change `driftedAccounts` to look up `computed[AccountKey{tenant, account.ID()}]`.
5. Add `TenantID` to `Drift` and `driftWire`. Additive JSON field; check the pact contract.
6. `EntriesFor` unscoped (`transactions.go:108`): a separate behaviour question for the product owner; not bundled into this fix.
7. Replace `TestVerifyBooks_UnscopedCallStaysPlatformWide` so it asserts the unscoped scope is passed to both reads, not that the legacy tenant is enumerated.
8. Update `fakes_test.go` and `pact_provider_test.go` to the new port shapes.

Mitigation until the fix lands (no code change): use `?tenant_id=` per tenant. The scoped path is correct (`transactions.go:184`).

## 4. Files affected

- `internal/app/ports/ports.go`
- `internal/app/usecases.go`
- `internal/adapters/postgres/transactions.go`
- `internal/adapters/postgres/accounts.go`
- `internal/adapters/http/handlers.go` (`driftWire`, `verdictBodyFor`)
- `internal/app/usecases_test.go`
- `internal/app/fakes_test.go`
- `internal/adapters/http/pact_provider_test.go`
- Possibly the pact contract files and the console TypeScript, if `tenant_id` is surfaced.

## 5. Risk assessment

- **Contract change.** The legacy-only enumeration was a deliberate step 03-01 guarantee. Single-tenant deployments are unaffected; multi-tenant ones go from silently incomplete to complete. Confirm the existing "unscoped keeps succeeding" acceptance scenarios still pass.
- **Newly surfaced drift.** The fix will detect real drift in non-legacy tenants. Correct, but the first run on production data may report new drift.
- **Performance.** The scan is already O(total history) (`usecases.go:626-630`). Adding `tenant_id` to the GROUP BY and a wider accounts read adds negligible cost. No index exists on `entries.tenant_id` alone; out of scope.
- **Port signature change** forces touching every fake and adapter. Mechanical and compile-checked.
- **Pact.** Adding a field is additive, but run the provider test (copy `libpact_ffi.so` to `/tmp` first).
- **Metrics.** `SetDriftedAccounts` may rise after the fix for the right reasons.

## 6. Regression test shape

- **Primary: acceptance layer.** Add scenarios to `tests/acceptance/multitenancy/milestone-03-verify-one-tenants-books.feature`, using the existing step vocabulary:
  - Given tenants A and B, each with a funded `treasury` and a `wallet-1`, with transfers on both.
  - When the operator calls `GET /health/trial-balance` with no tenant_id.
  - Then "Books balance: YES" and `drifted` is empty.
  - Variant 1 (non-legacy tenants only, catches the false negative): corrupt tenant B's stored balance via the `alter_stored_balance` hook. The check reports NO and names tenant B's `treasury` only.
  - Variant 2: the legacy tenant plus one named tenant sharing the id, reproducing the reported -100/-200 case.
  - Scenarios must carry the contract-shape tags; dispatch via the acceptance designer agent.
- **Secondary: unit, `internal/app/usecases_test.go`.** Fake store with two tenants and one shared id. Assert no drift when each (tenant, account) matches, and exactly one drift row when one is off. Good fit for a property-based test: for any partition of balanced entries across N tenants with colliding ids, the unscoped drift set is empty.
- **Adapter layer: `internal/adapters/postgres`** (next to `trial_balance_tenant_scope_test.go`). Assert `ComputedBalances(Unscoped())` returns two distinct keys for the shared id with -100 each, not one -200.
- Run with `go test -count=1`, in the foreground.

## 7. User decisions (2026-10-16)

- Root cause and fix direction approved: tenant-qualified (tenant_id, account_id) keys on the platform-wide path; remove the legacyTenantID pin in VerifyBooks; add TenantID to Drift / driftWire.
- EntriesFor unscoped (transactions.go:108) is OUT OF SCOPE. Track it separately; do not change it.
- Single-tenant behaviour must be unchanged.
