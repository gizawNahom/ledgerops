# RED classification — multitenancy DISTILL

Per `nw-distill` § Pre-DELIVER fail-for-the-right-reason gate. Run:

```
LEDGEROPS_AT_TAGS="" go test ./tests/acceptance/multitenancy/... -run TestMain -v
```

against this project's real toolchain (Docker + Testcontainers available in
this environment) on 2026-09-03, all 18 scenarios enabled (including
`@pending` ones, per the gate's own instruction — the escape hatch
`suite_test.go` documents for exactly this run). Result: **18 scenarios, 8
passed, 10 failed — zero `IMPORT_ERROR`/`FIXTURE_BROKEN`/`SETUP_FAILURE`, zero
`WRONG_ASSERTION`/`OBSERVABLE_NOT_AT_PORT`.** Every failure is
`MISSING_FUNCTIONALITY`. Gate: **PASS** — genuine RED, not BROKEN.

## Failing scenarios (all `MISSING_FUNCTIONALITY`)

| Scenario | File | Reason |
|---|---|---|
| Provisioning a tenant issues a scoped credential | milestone-01 | `POST /tenants` is still `scaffold("provision_tenant")` (router.go) — 501, no `ProvisionTenant` use case exists yet |
| Two tenants can be provisioned independently | milestone-01 | Same — depends on the same scaffold |
| A duplicate tenant name is refused | milestone-01 | Scaffold returns `__SCAFFOLD__`, not `tenant_already_exists` — I10 uniqueness check does not exist yet |
| Two tenants independently reuse the same account name | milestone-02 | Cascades from provisioning being unbuilt: neither tenant ever receives a real `tenant_key`, so `POST /accounts` refuses both as `unidentified_caller` |
| A tenant's account is invisible to every other tenant | milestone-02 | Same cascade — refused today as `unidentified_caller` (401), not yet the eventual `account_not_found` (404) I8 will produce once tenant scoping exists |
| A tenant's entries are scoped to that tenant alone | milestone-02 | Same cascade |
| The existing unscoped entries call keeps working for the console's own credential | milestone-02 | `alice` was never actually opened (the funding `OpenAccount` call itself failed for the same cascade reason), so the console-compat read 404s on a genuinely absent account |
| A tenant's drift is named without exposing other tenants | milestone-03 | `GET /health/trial-balance?tenant_id=` still ignores the query parameter — today's single-tenant `VerifyBooks` answers a platform-wide `YES` regardless of which tenant is asked about |
| Checking an unprovisioned tenant is refused | milestone-03 | Same — no `tenant_not_found` check exists yet; an unrecognized `tenant_id` is silently accepted |
| A newly onboarded tenant operates invisibly to an existing tenant (walking skeleton) | walking-skeleton | Funding `alice` failed for the provisioning cascade reason above, so the balance reads 0.00 instead of 100.00 |

## Passing scenarios (legitimately green already)

The 8 passes are every scenario whose refusal is already produced by
**existing, unmodified** production code — `requireOperatorKey` already
refuses a non-admin/unissued/missing bearer token today, and this feature
reuses that middleware unchanged (DDD-22) for `POST /tenants`. These are not
false positives: they prove D8/DDD-22's "reuse `requireOperatorKey` verbatim"
decision already holds for the one new route, before any tenant-specific code
exists. Slice 01's own scaffold route sits behind this middleware, so the
non-admin/unissued-credential provisioning scenarios in milestone-01 pass
today and will continue to pass unchanged through DELIVER.

## Cascade note for DELIVER

Slice 01 (provisioning) is a hard dependency of every slice-02/03 scenario, as
the slice briefs already state. This run makes that dependency observable in
the RED signal itself: 7 of the 10 failures are really "provisioning doesn't
exist yet" surfacing through downstream scenarios, not 7 independent gaps.
DELIVER's own one-at-a-time sequence (walking skeleton first, milestone-01
next) will collapse most of this cascade before milestone-02/03 are attempted
— consistent with the slice dependency chain 01→02→03 already recorded in
`feature-delta.md` § Wave: DISCUSS / Wave decisions summary.
