# Upstream issues found by DISTILL — container-image-release

Written by `nw-acceptance-designer` (DISTILL) on 2026-10-19. DISCUSS, DESIGN and DEVOPS sections of `feature-delta.md` were not edited.

Status: DISTILL STOPPED at the Wave-Decision Reconciliation hard gate. No scenario, step definition or scaffold has been written. Item U-1 is blocking. Items U-2 to U-9 are specification gaps DISTILL will resolve as stated unless told otherwise.

## U-1 (BLOCKING) `GET /health` does not exist

Every wave treats `GET /health` as an existing, unauthenticated route that answers 200:

- DISCUSS: D8 (smoke "checks `/health`"), US-2 (scenarios "A correctly configured image starts", "The demo tenant key is optional" expect `/health` 200), US-3 (smoke requires `/health` 200), journey `pull-and-run-ledgerops.yaml` S4 ("`GET /health`").
- DESIGN: § Driving ports lists `GET /health, GET /console, GET /console/verdict` as "HTTP (existing) | Unchanged".
- DEVOPS: `scripts/smoke` requires `/health` 200 (KPI-R1); § Observability stack proposes a Dockerfile `HEALTHCHECK` against `/health`.
- Slice 03: "require `/health` 200".

The code has no such route. Evidence (2026-10-19, branch `feature/container-image-release`):

- `internal/adapters/http/router.go` registers only `GET /metrics` (unauthenticated), then, behind `requireOperatorKey`, `POST /tenants`, `GET /health/trial-balance`, `GET /console/verdict`, `POST /tenant-links`, `DELETE /tenant-links/{link_id}`; behind the tenant key `POST /accounts`, `POST /transfers`, `POST /counterparties`, `GET /accounts/{id}`, `GET /accounts/{id}/entries`, `GET /transfers/{transfer_id}`; plus `GET /console` and `GET /console/*` when a bundle exists.
- `grep -rn '"/health"'` over `*.go`, `*.yml`, `Makefile`, `*.md` finds nothing outside the feature's own docs. The Makefile's `wait-app` target probes `GET /accounts/__probe__` with the demo tenant key instead.
- The only `/health…` route is `/health/trial-balance`, which needs the operator key.

Why it blocks: it is the observable of the walking skeleton ("start it, `/health` OK"), of US-2's happy path, of the release smoke (KPI-R1) and of the quickstart. DISCUSS § Out-of-scope also forbids "changing … any API behavior", so adding the route is a scope decision, not an implementation detail.

Decision needed (owner: user, with DESIGN to record it):

- Option A (recommended): add an unauthenticated `GET /health` answering 200 with a fixed body (no secrets, no database access beyond what startup already proved). It is a new API surface and needs an amendment to DISCUSS "Out-of-scope", a DESIGN driving-port row (new, not existing) and, optionally, the DEVOPS `HEALTHCHECK`. DISTILL would register it with the repository's RED scaffold idiom (`scaffold("health")` in `router.go`, answering 501 `__SCAFFOLD__`) so the walking skeleton fails for the right reason, and state the contract (status 200, no authentication, no body secrets) in the scenarios.
- Option B: keep the API as it is and redefine "healthy" as an existing route, for example `GET /health/trial-balance` with the operator key answering 200 (zero imbalance), or `GET /metrics`. Then US-2, US-3, the journey, KPI-R1 and the DEVOPS text all need their wording corrected, and "healthy" would depend on the operator key being known to the smoke harness (it is generated there, so this is feasible). `/metrics` proves only that the process is up, not that the database is reachable.

DISTILL cost of either choice is small once decided: the route is a single constant in the suite's domain types.

## U-2 (non-blocking, needs a decision) Moved version tag: DISCUSS says "refuses", DESIGN/DEVOPS resume

- DISCUSS US-3, "A published version is never silently overwritten": "When the tag `v0.2.0` is moved to another commit and pushed, Then the pipeline refuses and the published digest is unchanged."
- DESIGN DDD-45 / ADR-020 item 4: if `X.Y.Z` already exists in GHCR the run resumes with that tag's digest (build skipped, `ci`, smoke and checks re-run, missing promotions completed). DDD-37's guard accepts "absent or same digest". DEVOPS `environments.yaml` `resume-from-tag` variant C reads: "`X.Y.Z` present but pointing at a different digest than **this run built**: promote refuses".
- Consequence: after a git tag is force-moved to a new commit, `classify` finds `0.2.0`, sets `resumed=true`, and the run re-validates the OLD digest and finishes green. Nothing is overwritten (the shared invariant holds), but the run does not "refuse", and the maintainer is not told the new commit was not released. Variant C can only occur in a race, not for a moved git tag.
- DISTILL will assert the shared invariant only (published digest unchanged, no version tag re-pointed) and cover the promote guard against a local `registry:2` ("tag exists with a different digest: refused, unchanged"). A workflow-level moved-tag scenario is `@requires_external`.
- Decision requested: should a resumed run also compare the image's `org.opencontainers.image.revision` label with the tagged commit and fail red on mismatch? (Recommended: yes, one line in `classify`/`smoke`; it makes DISCUSS's "refuses" true.)

## U-3 Tag grammar boundaries are unspecified (C1b)

D2 says "strict `vMAJOR.MINOR.PATCH`, prerelease suffix allowed". Not specified: leading zeros (`v01.2.3`), build metadata (`v1.2.3+b5`), an empty prerelease (`v1.2.3-`), a capital `V`, numeric overflow. DISTILL will cover only the specified cases (`v0.2.0`, `v0.2.0-rc.1` accepted; `v1.2`, `1.2.3`, `v1.2.3.4`, `nightly-2026-10-17` rejected) and leaves the rest to DELIVER's unit table, which should pin them.

## U-4 Minor-tag rule for an older patch released after a newer one

US-6 states: `latest` moves only for the highest stable version; a patch moves the minor tag; older exact tags never move. It does not say what `0.2` does when `v0.2.1` is released after `0.2.2` exists. DISTILL will assert the natural reading as a property (a minor tag never moves to a lower patch than it already points at). Confirm or amend.

## U-5 Output vocabulary is illustrative

DISCUSS marks all message wording "illustrative". DISTILL needs stable tokens to assert on and will keep them in one place (`domain_types.go`, so wording changes touch one file): `schema at version <n>`, `app role password set` / `updated`, `no pending migrations`, `no changes applied`, `password was not changed`, `config invalid`, the variable names, the privilege names (`CREATEROLE`, `ADMIN OPTION`, `CONNECT`, `CREATE` on schema `public`, table owner), `DROP ROLE ledgerops_app`, `dirty`. DELIVER may change wording provided the tokens are updated in that one place.

## U-6 Exit codes and usage errors

US-2/D9 fix exit 1 for a serve configuration refusal. Migrate refusals are "non-zero". An unknown subcommand ("usage error", ADR-018) has no stated exit code or text. DISTILL asserts only non-zero plus naming `serve`, `migrate` and `version`.

## U-7 "Killed between two migration files" cannot be produced by an external kill

golang-migrate sets the dirty flag before each file, so a real kill lands inside a file or in a window of microseconds. DISTILL produces that state through the step-wise entry (`postgres.MigrateStep`, ADR-017 "orchestration entry points") and asserts its observable result (role NOLOGIN, version clean, re-run completes). The mid-file kill is real: an event trigger holds a chosen `CREATE TABLE` for a few seconds, then the test calls `pg_terminate_backend` and kills the process. Note for DELIVER: `MigrateStep`'s signature will change with the password parameter; the one call site in the suite's world is what to update.

## U-8 Promote logic: DISTILL-imposed environment contract

DEVOPS OPS-29 keeps `promote` free of repository code, so its guards are inline shell in `release.yml`. To test them against a local `registry:2`, DISTILL will run the job's `run:` steps with these environment variables supplied by the test and requires that the steps read their inputs only from them: `IMAGE_REPO`, `IMAGE_DIGEST`, `PLAN_VERSION`, `PLAN_TAGS`, `PLAN_MOVE_LATEST`, `PLAN_HIGHEST_STABLE`. DEVOPS/DELIVER may rename them, provided `domain_types.go` is updated. The same applies to `scripts/smoke` flags (`--image`, `--postgres-image`) and its summary line `smoke_pull_to_healthy_seconds=<n> arch=<a>` (KPI-R1).

## U-9 Test-harness facts DELIVER must know

- Existing suites keep compiling until `postgres.Migrate`/`MigrateStep` change signature; DISTILL does not change them.
- `go build -overlay` is used to build `console_embed` variants without touching the working tree.
- The frozen pre-feature fixture is `internal/adapters/postgres/prefeature/` (already created, pinned by checksum, import-guarded). `TestLiveZeroOneIsPinnedOnceEdited` is skipped until slice 01a edits `0001`; that slice must pin the new checksum and delete the skip.
