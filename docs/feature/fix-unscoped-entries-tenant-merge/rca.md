# RCA: operator's unscoped GET /accounts/{id}/entries merges the entries of every tenant sharing an account id

Method: static analysis only. Not reproduced live (a live repro needs a new test). Sibling of `docs/feature/fix-trial-balance-tenant-collision/rca.md` (follow-up (a) in `docs/evolution/2026-10-16-fix-trial-balance-tenant-collision.md`).

## 1. Root cause chain

PROBLEM: with tenants A and B each holding `treasury`, `GET /accounts/treasury/entries` with the operator key and no `tenant_id` returns both tenants' rows interleaved, with nothing to tell them apart. The console drill-down shows them as one account's history.

**WHY 1 (symptom).** The response is a flat `{"entries":[...]}` list with no tenant attribution, and `running_balance` is folded across both tenants.
- `entriesToWire` (`internal/adapters/http/handlers.go:306-318`) emits only `transaction_id`, `counterparty`, `amount`, `recorded_at`, `running_balance`.
- Domain `Entry` (`internal/domain/post.go:205-212`) has no tenant field.
- `runningBalances` (`internal/app/usecases.go:571-590`) is one cumulative sum over all rows returned.

**WHY 2 (the query is not tenant-bounded on the operator path).**
- `EntriesFor` unscoped (`internal/adapters/postgres/transactions.go:107-110`): `WHERE account_id = $1 ORDER BY recorded_at, sequence`. It neither filters on nor selects `tenant_id`.
- The scoped branch (lines 103-105) is correct: `WHERE tenant_id = $1 AND account_id = $2`.
- `scanAccountEntries` (`transactions.go:250-271`) has no column to scan a tenant into.

**WHY 3 (the operator cannot pick a tenant; port and use case cannot carry one).**
- `requireTenantKeyOrOperatorKey` (`router.go:216-219`, `:390-410`) injects `Unscoped()` for every operator-key caller. `getEntriesHandler` (`handlers.go:284-304`) reads only that context scope.
- `/health/trial-balance` and `/console/verdict` accept `?tenant_id=` via `trialBalanceScopeFromQuery` (`handlers.go:803-808`); the entries route has no equivalent.
- Port: `EntriesFor(ctx, scope, accountID) ([]domain.Entry, error)` (`ports.go:162`) cannot express a (tenant, entry) pair. `TracedEntry` (`usecases.go:511-514`) has no tenant.
- `GetEntries` unscoped existence check is the deliberately tenant-agnostic `ExistsAnyTenant` (`accounts.go:120-129`).

**WHY 4 (design allowance).**
- Steps 02-02 and 02-04 kept the unscoped entries read "byte-identical to the pre-multitenancy query" as a console-compatibility constraint (`usecases.go:530-541`, `router.go:209-215`, `ports.go:74-79`). That holds only while all rows belong to one tenant; migration 0003 made account identity `(tenant_id, id)`.
- The trial-balance fix introduced `AccountKey` (`ports.go`) but `EntriesFor` was logged out of scope (trial-balance rca.md §2, §7; `milestone-03b-...feature:17`).

**WHY 5 (root causes).**
- ROOT CAUSE A (design/code): the unscoped entries read model is keyed by `account_id` alone; no tenant-qualified identity exists on the entries port, use case, wire shape.
- ROOT CAUSE B (API surface): the operator cannot pass `tenant_id` to the entries endpoint, unlike the sibling operator reads.
- ROOT CAUSE C (consumer): the console has no tenant identity to carry. `DriftRow` (`web/console/src/testing/domainTypes.ts:10-15`) lacks `tenantId`; `parseDriftRow` (`apiClient.ts:67-88`) drops the `tenant_id` the server has emitted since `c8b9fb7`; `fetchEntries(accountId)` (`apiClient.ts:19, 208-211`), `selectAccount` (`ConsoleApp.tsx:119-125`) and `DriftTable` (keys by `row.accountId`) all use account id alone.
- ROOT CAUSE D (process): tests pin the merge as the contract: `TestTransactionRepository_EntriesFor_UnscopedSeesEveryTenantScopedSeesOnlyItsOwn` (`postgres/transactions_test.go:129-191`, "want 2 ... pre-existing unscoped contract"); fakes' `EntriesFor` ignore scope (`internal/app/fakes_test.go:345-353`, `pact_provider_test.go:453-461`); no acceptance scenario has shared ids on the entries read.

Backward validation: A produces untagged merged rows; B makes the merge unavoidable for the operator; C means a correct server still yields a merged drill-down; D explains why it went unseen. All four are needed for the stated expectation; none contradict.

## 2. Contributing factors

- `running_balance` is meaningless in the merged view; US-3 forbids client-side recomputation (`domainTypes.ts:30-31`), so client-side filtering cannot fix it.
- Unscoped ordering (`recorded_at, sequence`) interleaves tenants; `sequence` is not unique across tenants.
- `DriftTable` row key and `ConsoleApp.tsx:96, 104` staleness checks compare `accountId` alone, so two tenants' drifting `treasury` rows would collide.
- Pact provider fake models a single tenant and ignores scope (follow-up (d)).

## 3. Proposed fix

Follow the trial-balance pattern: tenant-qualified identity, additive `tenant_id`, `?tenant_id=` narrowing for the operator.

1. Port (`ports.go:162`): `type TenantEntry struct{ TenantID string; Entry domain.Entry }`; `EntriesFor(...) ([]TenantEntry, error)`. The pure `domain.Entry`/`NewEntry` stay untouched (ADR-007).
2. Postgres (`transactions.go:96-116, 250-271`): select and scan `tenant_id` in both branches; unscoped order `ORDER BY tenant_id, recorded_at, sequence`.
3. Use case (`usecases.go:511-590`): `TracedEntry.TenantID`; `runningBalances` folds per tenant (reset sum and currency when tenant changes). Single-tenant output numerically identical.
4. HTTP (`handlers.go:284-318`): `entriesToWire` emits `tenant_id` for all callers. When the context scope is `Unscoped()` (operator) and `?tenant_id=` is present, use `ScopedToTenant`. A tenant-key caller sending `?tenant_id=` naming a different tenant is refused through the existing cross-tenant refusal; it never widens or switches tenant.
5. Console: `DriftRow.tenantId`; `parseDriftRow` reads `tenant_id`; `fetchEntries(accountId, tenantId?)` appends `?tenant_id=` (URL-encoded); `DriftTable` passes `(accountId, tenantId)`, keys by tenant+account, shows a Tenant column; `ConsoleApp` selection holds both and compares both; `EntryTrace` shows the tenant and fallback URL. No client-side filtering.
6. Tests (foreground, `go test -count=1`; copy `libpact_ffi.so` to `/tmp` before `internal/adapters/http`): replace the adapter pin with per-tenant attribution; use-case property test for per-tenant running balances with colliding ids; acceptance scenarios (contract-shape tagged, via `nw-acceptance-designer`): unscoped read names each tenant, narrowed read returns one tenant with correct running balances, tenant key cannot reach another tenant; console component test with two drifted `treasury` rows from different tenants; update fakes and pact provider fake to the new port and honour scope; remove the "out of scope by decision" note in `milestone-03b-...feature:17`.

Mitigation until the fix lands (no code change): read via a tenant key; the scoped path is correct.

## 4. Files affected

- `internal/app/ports/ports.go`, `internal/app/usecases.go`
- `internal/adapters/postgres/transactions.go`, `internal/adapters/http/handlers.go`
- Tests/fakes: `internal/adapters/postgres/transactions_test.go`, `internal/app/fakes_test.go`, `internal/app/usecases_test.go`, `internal/adapters/http/pact_provider_test.go`, `tests/acceptance/multitenancy/*` (feature, steps, world)
- Console: `web/console/src/testing/domainTypes.ts`, `apiClient.ts`, `components/DriftTable.tsx`, `ConsoleApp.tsx`, `EntryTrace.tsx` and tests; `web/console/pacts/console-ledgercore.json` if the consumer pact gains the query
- Docs: `docs/product/kpi-contracts.yaml` and brief amendment if a byte-identical scenario is hit

## 5. Risk assessment

- Contract: additive field plus one new optional query parameter. The consumer pact matches `$.entries` by type, so existing interactions are unaffected; a new interaction and provider state are needed if the console sends `?tenant_id=`.
- Byte-identical scenarios (console-compat `milestone-02:69-73`, brief's CI-gated scenarios) may be hit by the new field; run them first. Any hit needs a KPI/brief amendment, as KPI-M4 had.
- Security: `tenant_id` must be honoured only for the operator scope; a regression test is required.
- Port change touches every fake/adapter (mechanical, compile-checked). Unscoped order becomes tenant-contiguous.
- Rollout: deploy the server before the console (old console ignores the field; new console against old server still shows the merge).
- Performance: negligible.

## 6. Classification

Code bug, not a design gap; no /nw-design escalation. The pattern (tenant-qualified keys, `TenantScope`, additive `tenant_id`) is already established. The two behaviour points below were confirmed by the user.

## 7. User decisions (2026-10-16)

- Root cause and fix direction approved: a tenant-qualified `TenantEntry` on the `EntriesFor` port, `tenant_id` selected and ordered by in postgres, `TracedEntry.TenantID`, `runningBalances` folded per tenant, `tenant_id` on `entriesToWire`, an operator-only `?tenant_id=` narrowing on `GET /accounts/{id}/entries`, and the console drill-down passing `tenantId` (no client-side filtering).
- Decision 1: the operator's no-tenant read returns every tenant's entries tagged with `tenant_id`, grouped by tenant, with `running_balance` restarting per tenant. It does not refuse with 409.
- Decision 2: `tenant_id` is emitted on each entry for all callers, including tenant-key callers. A tenant-key caller that sends a `?tenant_id=` naming a different tenant is refused through the existing cross-tenant refusal.
- Single-tenant behaviour is unchanged apart from the additive `tenant_id` field. Any byte-identical scenario hit by that field needs a KPI/brief amendment, as KPI-M4 had.
- Out of scope: follow-ups (b), (c) and (e) from the trial-balance evolution doc.
