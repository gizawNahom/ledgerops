# Feature Delta — container-image-release

Density: `lean` (Tier-1 `[REF]` only). Expansions available on request via `--expand <id>`.

Publishes a versioned, signed, multi-arch ledgerops container image to GHCR on
every release tag, so a client can `docker pull` and run it instead of building
from source. The image carries its own migration command and takes the
application database password from configuration instead of a literal baked
into SQL. Slices are numbered in delivery order; briefs are in `slices/`.

Evidence basis: founder-estimated, like every job in `docs/product/jobs.yaml`.
No DISCOVER or DIVERGE wave ran for this feature. Missing DIVERGE is recorded
as a risk: the registry, tag, signing, and multi-arch choices below were made by
the user in a Q&A round (D1-D9), not weighed against researched alternatives.

Prior Wave Consultation — reading confirmation

- ✓ `docs/product/jobs.yaml`, `docs/product/vision.md`
- ✓ `docs/product/personas/integrating-developer.yaml`, `platform-operator.yaml`
- ✓ `docs/product/architecture/brief.md` § Deployment shape (lines 61-94)
- ✓ `docs/product/journeys/verify-the-books.yaml` (format precedent; the four existing journeys cover posting, verifying and tenancy, none covers install or run)
- ✓ `docs/feature/inter-tenant-transfer/feature-delta.md` and `slices/slice-01-*.md` (format precedent)
- ✓ Packaging surface: `Dockerfile`, `docker-compose.yml`, `.github/workflows/ci.yml` and `nightly.yml`, `Makefile`, `README.md`, `cmd/api/main.go`, `internal/adapters/postgres/store.go`, `migrations/0001_init.up.sql`, `internal/adapters/http/console_static.go`, `router.go`
- ⊘ `docs/feature/container-image-release/discover/`, `diverge/` (not found)
- ⊘ `docs/project-brief.md`, `docs/stakeholders.yaml` (not found)
- ⊘ `docker-compose.loadtest.yml`, `load-test.yml` (grep only; they hardcode `app-secret` and call `docker compose up`)

Brownfield finding (current state, with refs)

- Image: `Dockerfile:3-14` builds only `./cmd/api` into `alpine:3.20`, entrypoint `ledgerops-api`. No version stamp, no OCI labels, no `.dockerignore`. No Node stage, so the console SPA is not in the image. `consoleDistDir = "web/console/dist"` is resolved against the process's working directory (`console_static.go:31`), `web/console/.gitignore` ignores `dist/`, and `mountConsole` silently registers nothing when `index.html` is absent (`console_static.go:45-49`). Copying the binary alone therefore yields a `/console` that 404s without any error.
- Migrations: the image has none. Compose runs a separate `migrate/migrate:v4.18.3` container with the SQL bind-mounted (`docker-compose.yml:33-44`). The same SQL is embedded in the Go package and `postgres.Migrate()` applies it (`store.go:41,69`), but only tests and acceptance harnesses call it; `cmd/api/main.go` never does.
- App DB password: `0001_init.up.sql:62` runs `CREATE ROLE ledgerops_app LOGIN PASSWORD 'app-secret'` inside an `IF NOT EXISTS` guard, so the password is hardcoded and never reset on an existing database. `0001_init.up.sql:67` hardcodes `GRANT CONNECT ON DATABASE ledgerops`. The literal `app-secret` is also in `docker-compose.yml:54`, `docker-compose.loadtest.yml`, and `testhelper_test.go:87`. The app already takes its password inside `LEDGEROPS_APP_DSN` (`main.go:42`), so the application side needs no change; the provisioning side does.
- Keys: `LEDGEROPS_OPERATOR_KEY` unset means `isOperatorKey` is always false (`router.go:291-293`), so every operator route answers 401 and the server still starts with no error (`main.go:58`). `LEDGEROPS_DEMO_TENANT_KEY` unset means the demo resolver is bypassed (`main.go:98-100`); it is optional today. `LEDGEROPS_APP_DSN` unset is not validated at all before `probeStartup`.
- Releases: `ci.yml` runs on `push` and `pull_request` for everything (`ci.yml:8-10`); no workflow reacts to a tag or publishes anything. `brief.md:73` already assumes rollback by "redeploying the previous image tag". README lines 181-182 list "no published container image; you build from source" as a limitation.

---

## Wave: DISCUSS / [REF] Persona ID

- P1 Integrating developer (`personas/integrating-developer.yaml`), primary for J11. Priya Nair, backend engineer at Kestrel Wallet, evaluating ledgerops for a points system, with Docker and no Go toolchain. Caveat inherited: archetype-unvalidated.
- P2 Platform operator (`personas/platform-operator.yaml`), primary for J12 and J13. Tomasz Kowalski, who runs Kestrel's ledgerops deployment and holds its secrets. Evidence: real-project-owner.
- Release maintainer (Gizaw, the project owner) acts in the Given steps of the publishing stories but is not a new persona; no story is written for the maintainer's own convenience.

## Wave: DISCUSS / [REF] JTBD one-liner

J11 (P1): When I want to try or run ledgerops for my product and I do not have a Go toolchain, I want to `docker pull` a versioned image and start it against my own Postgres with a password I chose, so I can integrate against a pinned, known release without building from source or running a well-known demo credential.

J12 (P2): When I put a published ledgerops image in front of platform funds, I want to verify it was built by the project's pipeline from the tagged source, so I never run a look-alike or tampered image.

J13 (P2): When I already run ledgerops from the demo stack or an earlier release, I want to move to a newer image and set my own app password in place, so I keep my ledger history and never rebuild or re-provision.

All three are added to `docs/product/jobs.yaml` (founder-estimated).

## Wave: DISCUSS / [REF] Locked decisions

User answers, taken as given (D1-D9) plus DISCUSS decisions (D10-D13).

- D1 Registry: GHCR, `ghcr.io/gizawnahom/ledgerops`, authenticated with `GITHUB_TOKEN`.
- D2 Release trigger: strict `vMAJOR.MINOR.PATCH` tags. Prerelease tags (`-rc.N` and similar) publish but never move `latest`. A GitHub Release is created per tag.
- D3 Image tags: `X.Y.Z`, `X.Y`, `latest` (stable only). OCI labels for version, revision, source. Version stamped into the binary.
- D4 Migrations: a `ledgerops migrate` subcommand in the same image, using the embedded SQL, run explicitly with the privileged DSN as a one-off or init container. Never auto-run at app startup; the app never holds the migrate role.
- D5 App password: `migrate` reads `LEDGEROPS_APP_PASSWORD` or `LEDGEROPS_APP_PASSWORD_FILE` and sets `ledgerops_app`'s password Go-side after the SQL migrations, on create and on rotate for existing databases (an explicit password always applies). Unset means migrate refuses; no fallback to `app-secret`. `0001`'s hardcoded `GRANT CONNECT ON DATABASE ledgerops` must work with `current_database()`. How to change an already-applied migration safely is a DESIGN question (OQ-2), not decided here.
- D6 Demo path: `docker-compose.yml`, the loadtest compose, the test helper and CI set `LEDGEROPS_APP_PASSWORD=app-secret` explicitly. Demo behavior unchanged; `make up` keeps building locally. A separate published-image run example (docker run / compose snippet) is documented.
- D7 First-release image scope: linux/amd64 and linux/arm64, the SPA console bundle served from the image, cosign keyless signing, SBOM attestation.
- D8 Release gate: the release workflow reuses `ci.yml` via `workflow_call` (full suite green), then a post-push smoke test pulls the published image, migrates a fresh Postgres and checks `/health`. Upgrade promise: databases that already ran the demo migrations with `app-secret` are supported and tested.
- D9 (revised 2026-10-17, user decision) The published image refuses to start when `LEDGEROPS_OPERATOR_KEY` or `LEDGEROPS_APP_DSN` is unset or blank (fail fast, one clear error, no baked-in demo keys). `LEDGEROPS_DEMO_TENANT_KEY` is optional: when unset the demo-tenant mapping is off and nothing resolves to `tnt_legacy_seed` through it. Rationale: the mapping is a demo affordance onto a sentinel tenant; a fresh production database has no legacy data, and requiring it would force every client to mint a credential for that tenant. The original D9 required all of the keys; only the demo key changed. OQ-1 closed.
- D10 Scope: oversized by the heuristic once D7 was chosen (see Scope Assessment). User-approved 2026-10-17 to keep as one feature with eight slices of at most one day each. OQ-11 closed.
- D14 First version is `v0.1.0`; release notes are GitHub auto-generated. OQ-12 closed.
- D11 Ordering: walking skeleton is slices 01-03 (migrate, safe start, gated publish with smoke), so a usable rc image exists after three slices. Hardening (upgrade, console, tags/arch, signing, docs) follows.
- D12 Two further jobs added beyond J11: J12 (provenance) and J13 (upgrade in place). Each is a distinct P2 outcome that the user's D7 and D8 choices imply.
- D13 One new journey, `docs/product/journeys/pull-and-run-ledgerops.yaml`, extends the SSOT; no existing journey changes.

## Wave: DISCUSS / [REF] Scope Assessment (Elephant Carpaccio Gate)

Result: oversized by heuristic, user-approved (2026-10-17) to keep as one feature with 8 slices of at most one day each.

| Signal | Reading |
|---|---|
| More than 10 stories | No: 8 |
| More than 3 modules or contexts | Yes: release CI, Dockerfile and packaging, postgres adapter, `cmd/api`, docs/README (5 touchpoints; the domain core is untouched) |
| Walking skeleton needs more than 5 integration points | No: Docker, Postgres, GHCR, Actions (4) |
| Estimated effort over 2 weeks | No: about 8 working days |
| Independent outcomes that could ship separately | Yes: supply-chain verification (slice 07) and arm64 have no dependency on the run-and-upgrade outcome |

Two signals fire. Mitigation: eight slices of at most one day, each shippable and demonstrable alone, ordered so an rc image is usable after slice 03. Recommended cut line, if the user wants a split: ship slices 01-06 and 08 as `container-image-release`, and move slice 07 (cosign and SBOM) plus the arm64 half of slice 06 to a follow-up feature. The user declined the split and kept the full D7 scope in this one feature (OQ-11 closed); the cut line above remains the fallback if delivery slips.

## Wave: DISCUSS / [REF] Story Map

User: P1 Priya (run it), P2 Tomasz (trust it, upgrade it). Goal: run a pinned, verified ledgerops release from `docker pull` against my own Postgres.

| Find a release | Pull | Prepare database | Start | Operate | Upgrade |
|---|---|---|---|---|---|
| Tag-triggered GitHub Release (06) | Pull exact tag, `docker pull` (03) | `migrate` with my password (01) | Fail-fast config check (02) | Console at `/console` (05) | Migrate existing demo DB in place (04) |
| Minor and `latest` tags (06) | Multi-arch manifest (06) | Non-default database name (01) | `version` output (05) | Verify signature and SBOM (07) | Rotate password (04) |
| Prerelease handling (06) | Anonymous pull (OQ-7) | | | Quickstart for published image (08) | |

Walking skeleton (thinnest end-to-end): US-1 (migrate with a password), US-2 (refuse bad config), US-3 (gated publish and smoke, amd64, exact tag only). After these, `docker pull` then `migrate` then run then `/health` works from a registry.

Releases by outcome:
- Release A, "A client can run a pinned image": US-1, US-2, US-3. Outcome KPIs K1, K3.
- Release B, "Existing and operator users are safe": US-4, US-5. KPIs K4, K6.
- Release C, "Looks like a real release": US-6, US-7, US-8. KPIs K2, K5, K7.

## Priority Rationale

1. Slice 01 first: the password literal in `0001` is the highest-uncertainty change (it touches an applied migration) and everything else depends on `migrate` existing. If setting a password Go-side breaks the append-only guarantee or the demo path, we learn it on day one.
2. Slice 02 second: tiny, no dependency, and it removes the failure that would otherwise make the smoke test in slice 03 meaningless (an image that starts with no operator key).
3. Slice 03 third: proves the pipeline end to end on an exact tag only. Biggest unknown in CI (workflow_call reuse of a compose-based `ci.yml`), so it comes before polish.
4. Slice 04 next: the upgrade promise (D8) protects people who already run the demo stack; it reuses 01's code and the 03 pipeline.
5. Slice 05: console and version. Independent of the others; placed after the pipeline exists so the smoke test can assert `/console`.
6. Slice 06: full tag set, arm64, labels, GitHub Release. Pure packaging polish over a working pipeline; moves `latest` for the first time.
7. Slice 07: signing and SBOM. Highest external-tooling risk (keyless identity, permissions) with the lowest effect on whether a client can run the image, so it comes after `latest` exists but before docs freeze the verify command.
8. Slice 08 last: documentation must name real tags and the real signing identity.

## Wave: DISCUSS / [REF] User stories with elevator pitches

Every story has a real user-invocable entry point and an observable result. No story is `@infrastructure`. Image and command names are illustrative; DESIGN settles exact names (OQ-3). Passwords below are fixtures, not secrets.

### US-1 — Migrate a fresh database with my own app password

job_id: J11 | persona: P1 | slice: 01

Problem: Priya Nair is a backend engineer at Kestrel Wallet who wants to try ledgerops against her own Postgres. The only way to set up the database today is the demo stack, whose migrate step carries a password written into the SQL (`app-secret`) that she cannot change, and that only works against a database literally named `ledgerops`.

#### Elevator Pitch
Before: Priya cannot prepare a database for ledgerops without accepting the hardcoded `app-secret` role password and the database name `ledgerops`.
After: run `docker run --rm -e LEDGEROPS_MIGRATE_DSN=postgres://kestrel_admin:...@db/kestrel_ledger -e LEDGEROPS_APP_PASSWORD_FILE=/run/secrets/ledgerops_app_password -v ./secrets:/run/secrets ledgerops:dev migrate` → sees `schema at version 6; app role password set; exit 0`
Decision enabled: Priya decides the database is ready and points her app DSN at `ledgerops_app` with the password she chose.

Domain Examples
1. Priya sets `LEDGEROPS_APP_PASSWORD='T7!kq@9x%Zm'` and migrates the empty database `kestrel_ledger`. Six migrations apply and `ledgerops_app` logs in with that exact password.
2. Tomasz Kowalski supplies the password as a Docker secret file `/run/secrets/ledgerops_app_password` whose content is `Zr4#mV8$wQ1\n`. The trailing newline is not part of the password.
3. Priya forgets to set a password at all (or sets it to the empty string). Migrate exits non-zero naming both variables and touches nothing; the database is still empty.

UAT Scenarios (BDD)

Scenario: A fresh database is ready with the password the operator chose
Given Priya has an empty Postgres 16 database `kestrel_ledger` and a privileged role `kestrel_admin`
And she sets the app password to `T7!kq@9x%Zm`
When she runs `migrate`
Then the command exits 0 and reports schema version 6
And `ledgerops_app` can log in with `T7!kq@9x%Zm`
And `ledgerops_app` is refused when it tries to UPDATE an entry
And the command's output does not contain `T7!kq@9x%Zm`

Scenario: The password can come from a mounted secret file
Given Tomasz's secret file contains `Zr4#mV8$wQ1` followed by a newline
When he runs `migrate` pointing at that file
Then `ledgerops_app` logs in with `Zr4#mV8$wQ1` and not with a value that includes the newline

Scenario: A secret file the image user cannot read gives a named error
Given Tomasz's secret file is owned by root with mode 0400 and the image runs as a non-root user
When he runs `migrate` pointing at that file
Then it exits non-zero naming the file and that it is not readable by the image user
And the database is unchanged

Scenario: Migration refuses to run without a password
Given Priya sets neither the password nor the password file
When she runs `migrate` against the empty database
Then the command exits non-zero naming `LEDGEROPS_APP_PASSWORD` and `LEDGEROPS_APP_PASSWORD_FILE`
And no table exists in the database afterwards
And `ledgerops_app` does not accept `app-secret`

Scenario: No moment exists when the app role accepts a password the operator did not supply
Given a migration run that is killed between two migration files
When someone tries to log in as `ledgerops_app` with `app-secret`
Then the login is refused

Scenario: A run killed during a migration file leaves a state migrate refuses to continue from
Given a migration run that is killed during a migration file, leaving a dirty version
When Priya re-runs `migrate`
Then it exits non-zero naming the dirty version and changes nothing
And `ledgerops_app` still refuses `app-secret`

Scenario: The first ledgerops database in a cluster can have any name
Given the cluster has no `ledgerops_app` role and the database is named `kestrel_ledger`, not `ledgerops`
When Priya runs `migrate`
Then it succeeds and `ledgerops_app` can connect to `kestrel_ledger`

Scenario: A second ledgerops database in the same cluster is refused
Given the cluster's database `kestrel_ledger` is already migrated, so the role `ledgerops_app` exists
When Priya runs `migrate` against a second, empty database `kestrel_ledger_2` in that cluster
Then it exits non-zero naming the conflict
And the role, its password, `kestrel_ledger` and `kestrel_ledger_2` are all unchanged
And running `migrate` again against `kestrel_ledger` itself (a database already carrying the schema) is not refused
And the message names the remedy: migrate the database that owns the role, use a separate cluster, or, when that database no longer exists, run `DROP ROLE ledgerops_app` as a privileged role

Scenario: Two simultaneous first migrations in one cluster cannot both claim the role
Given two empty databases `kestrel_ledger` and `kestrel_ledger_2` in one cluster
When two `migrate` runs start at the same time
Then exactly one succeeds and the other refuses with the cluster-rule message
And neither database is left in a dirty migration state

Scenario: A mismatched password is caught at app start, not at the first request
Given the database was migrated with `T7!kq@9x%Zm`
When Priya starts the app with an app DSN carrying a different password
Then the app exits non-zero with a startup refusal naming the role and that authentication failed
And the log line does not contain either password

Acceptance Criteria
- [ ] `migrate` applies the full migration set to an empty database and sets the app role password from `LEDGEROPS_APP_PASSWORD` or its `_FILE` variant; a trailing newline in the file is not part of the password
- [ ] `migrate` without a usable password (unset, empty) exits non-zero before changing the database and names both variables
- [ ] At no point, including after an interrupted run, does `ledgerops_app` authenticate with a password the operator did not supply
- [ ] A run killed during a migration file leaves a dirty version; re-running `migrate` refuses naming that version and changes nothing
- [ ] A password file the image user cannot read fails with a named error and no database change
- [ ] Migration works for a database with any name, for the first ledgerops database in a cluster
- [ ] A fresh `migrate` against an empty database in a cluster where `ledgerops_app` already exists exits non-zero naming the conflict, with the role, its password and both databases unchanged; a database already carrying the ledgerops schema (upgrade, rotation, re-run) is not refused; the message names the remedies (migrate the owning database, use a separate cluster, or `DROP ROLE ledgerops_app` as a privileged role when the owning database no longer exists)
- [ ] Two simultaneous `migrate` runs against two empty databases in one cluster: exactly one succeeds, the other refuses with the cluster-rule message, and neither database is left dirty
- [ ] The password never appears in command output or logs, with or without failure
- [ ] `docker-compose.yml`, the loadtest compose, the test helper and CI set `LEDGEROPS_APP_PASSWORD=app-secret` explicitly and `make demo-01..04`, `chaos-01`, `race-02`, `race-03` pass unmodified
- [ ] An app started with the wrong password exits non-zero with a clear refusal and no password in the log

Outcome KPIs
- Who: integrating developers preparing a database from the image
- Does what: reach a migrated database with their own password without editing SQL or choosing the database name `ledgerops`
- By how much: 100% of fresh-database acceptance runs end with the supplied password working and `app-secret` refused
- Measured by: acceptance suite against a Testcontainers Postgres, and the release smoke test
- Baseline: 0% (the password is fixed in SQL)

Technical Notes
- Changes an already-applied migration's content (`0001`): safe handling is OQ-2. The compose `migrate` service moves from `migrate/migrate` to this subcommand, because `migrate/migrate` cannot set the password; demo CI then exercises the same code the release ships.
- The privileged role needs enough privilege to create and alter roles (OQ-8). `migrate` runs a preflight privilege probe first: an insufficient privilege fails before any schema change with "no changes applied".
- One ledgerops database per Postgres cluster (user decision, DESIGN review cycle 1): the role `ledgerops_app` is cluster-global, so a second fresh database in the same cluster is refused. Remedy: use a separate cluster, migrate the database that already owns the role, or, when that database no longer exists, run `DROP ROLE ledgerops_app` as a privileged role. The role claim is atomic (DDD-48), so concurrent first migrations cannot both succeed.
- A secret file owned by root with mode 0400 is unreadable by the non-root image user; the named error documents this trap.
- Dependencies: none inside this feature; depends on shipped `ledger-core`.

### US-2 — Know immediately when the image is misconfigured

job_id: J11 | persona: P1 | slice: 02

Problem: Priya starts the image with only a database DSN. Today the server starts normally and answers 401 on every operator route, because an unset `LEDGEROPS_OPERATOR_KEY` makes no token valid (`router.go:291-293`); she finds out only when `POST /tenants` fails, and cannot tell whether the key, the route or the build is at fault. An unset `LEDGEROPS_APP_DSN` is not validated either.

#### Elevator Pitch
Before: a misconfigured image starts and then fails confusingly at the first operator call, or points at the wrong database.
After: run `docker run --rm ghcr.io/gizawnahom/ledgerops:0.1.0` with no settings → sees `config invalid: LEDGEROPS_APP_DSN is required; LEDGEROPS_OPERATOR_KEY is required` and exit 1 (wording illustrative)
Decision enabled: Priya decides which variables to set before retrying, with every problem listed at once.

Domain Examples
1. Priya sets only the operator key; the error names `LEDGEROPS_APP_DSN` and the process exits 1 without opening a database connection.
2. Tomasz sets the DSN and operator key but no demo tenant key; the app starts, `/health` answers, and the demo-tenant mapping is off.
3. A deploy template leaves `LEDGEROPS_OPERATOR_KEY=` empty; it is treated as unset and refused, not accepted as an empty credential.

UAT Scenarios (BDD)

Scenario: Missing operator key is refused at start with a clear message
Given Priya starts the image with a valid app DSN and no operator key
When the container starts
Then it exits 1 and the message names `LEDGEROPS_OPERATOR_KEY`
And no connection to the database was attempted

Scenario: All missing settings are reported together
Given no operator key and no app DSN are set
When the container starts
Then one message names both variables

Scenario: The demo tenant key is optional and its mapping stays off when unset
Given Tomasz sets the DSN and operator key but not `LEDGEROPS_DEMO_TENANT_KEY`
When the container starts and a caller presents the bearer token `demo-tenant-key`
Then the container starts and `/health` answers 200
And that token is refused 401 and resolves to no tenant, including `tnt_legacy_seed`

Scenario: A blank demo tenant key counts as unset
Given `LEDGEROPS_DEMO_TENANT_KEY` is set to an empty string
When the container starts
Then it starts normally with the demo-tenant mapping off

Scenario: A set demo tenant key still works for the demo stack
Given the demo compose sets `LEDGEROPS_DEMO_TENANT_KEY` to `demo-tenant-key`
When a caller presents `demo-tenant-key`
Then it resolves to `tnt_legacy_seed` exactly as before

Scenario: Empty and whitespace-only values count as unset
Given `LEDGEROPS_OPERATOR_KEY` is set to an empty string
When the container starts
Then it is refused as missing

Scenario: A correctly configured image starts
Given the DSN and operator key are set and the database was migrated
When the container starts
Then `/health` answers 200 and `/tenants` accepts the operator key

Scenario: The serving process refuses to hold the migration credential
Given the container environment contains `LEDGEROPS_MIGRATE_DSN`
When the container starts to serve
Then it exits 1 naming `LEDGEROPS_MIGRATE_DSN`
And no database connection was attempted

Scenario: Key values are never printed
Given Tomasz sets `LEDGEROPS_OPERATOR_KEY` to `op-9f3b71c2`
When the container starts, successfully or not
Then `op-9f3b71c2` appears in no log line

Acceptance Criteria
- [ ] Unset or blank `LEDGEROPS_OPERATOR_KEY` or `LEDGEROPS_APP_DSN` causes exit 1 before any database connection (D9 as revised)
- [ ] Unset or blank `LEDGEROPS_DEMO_TENANT_KEY` is accepted and turns the demo-tenant mapping off; no token resolves to `tnt_legacy_seed` through it
- [ ] One message lists every missing required variable
- [ ] Key and DSN values never appear in logs
- [ ] `serve` refuses to start when `LEDGEROPS_MIGRATE_DSN` is present in its environment, naming the variable, before any database connection
- [ ] With all variables set, the server starts and existing behavior is unchanged
- [ ] The demo compose sets all three so `make up` is unaffected

Outcome KPIs
- Who: developers and operators running the image
- Does what: see a configuration error at start, not at the first operator call
- By how much: 100% of missing-variable acceptance runs refuse at start, 0 start silently
- Measured by: acceptance suite (process exit status and message)
- Baseline: 0% (missing operator key starts silently)

Technical Notes
- OQ-1 (closed, user decision 2026-10-17): `LEDGEROPS_DEMO_TENANT_KEY` is optional. Current code already behaves this way (`main.go:98-100`), so no code change is needed for it; the work in this story is only the required-variable check for the other two.
- The check belongs at the entrypoint; the domain core is untouched.

### US-3 — Pull a pinned release from GHCR

job_id: J11 | persona: P1 | slice: 03

Problem: Priya needs a version of ledgerops she can pin and re-pull, and the maintainer needs confidence that what lands in the registry actually runs. Today no image is published; CI never reacts to a tag and nothing proves a built image can migrate and serve.

#### Elevator Pitch
Before: Priya cannot get ledgerops without cloning the repository and building it.
After: run `docker pull ghcr.io/gizawnahom/ledgerops:0.1.0` → sees `Status: Downloaded newer image for ghcr.io/gizawnahom/ledgerops:0.1.0`
Decision enabled: Priya decides to adopt `0.1.0` as her pinned version and builds her deployment on it.

Domain Examples
1. Gizaw pushes `v0.1.0` on a green commit. After the pipeline finishes, `0.1.0` is pullable anonymously and the smoke check passed.
2. The integration job fails on the commit tagged `v0.1.1`. No image is published and the run is red on the failing job.
3. Gizaw pushes `nightly-2026-10-17` and the branch `fix-typo`. Nothing is published.

UAT Scenarios (BDD)

Scenario: A release tag publishes a pullable image
Given main is green at the commit tagged `v0.1.0`
When the release pipeline finishes
Then `ghcr.io/gizawnahom/ledgerops:0.1.0` can be pulled without logging in

Scenario: A red build never publishes
Given an integration test fails at the commit tagged `v0.1.1`
When the pipeline runs
Then no image exists for `0.1.1` and the run is red

Scenario: A pushed image is proven usable before it gets a version tag
Given the image for `0.1.0` has been pushed by digest and no version tag exists yet
When the pipeline's check pulls that digest for each published architecture, migrates a fresh Postgres and starts it
Then `/health` answers 200 and only then is `0.1.0` tagged

Scenario: A version that fails its usability check is not presented as a release
Given the pushed image cannot migrate a fresh database
When the check fails
Then the run ends red naming the failing step and no version tag exists for it

Scenario: Only strict version tags publish
Given Gizaw pushes the tag `v1.2`, the tag `nightly-2026-10-17`, and a branch
When the pipelines run
Then no image is published for any of them

Scenario: A published version is never silently overwritten
Given `0.1.0` is already published
When the tag `v0.1.0` is moved to another commit and pushed
Then the pipeline refuses and the published digest is unchanged

Acceptance Criteria
- [ ] Pushing a tag `vMAJOR.MINOR.PATCH` (prerelease suffix allowed) publishes `ghcr.io/gizawnahom/ledgerops:<version without v>` for amd64
- [ ] Publishing requires the full existing CI suite to pass on that commit
- [ ] Anonymous pullability is checked on the pushed digest before any version tag is promoted; the first release run is expected to fail this check until the owner makes the GHCR package public (OQ-7), after which a re-run passes
- [ ] The check pulls the image by digest before any version tag exists, for each published architecture (amd64 in this slice; arm64 joins in slice 06), migrates a fresh Postgres with a generated password, starts it, and requires `/health` 200; the version tag appears only after the check passes (and, once slice 07 lands, after signature verification)
- [ ] A failed check leaves the run red and no version tag exists for it
- [ ] Non-matching tags and branch pushes publish nothing
- [ ] An existing version tag is never overwritten

Outcome KPIs
- Who: integrating developers pulling the image
- Does what: run a pinned image from the registry without a source checkout
- By how much: smoke check from pull to healthy in at most 120 seconds on the CI runner; 100% of published versions pass the check
- Measured by: release workflow summary (seconds and result)
- Baseline: not possible today

Technical Notes
- Pre-requisite (OQ-7): the first push creates a private package; the owner must set it public once for anonymous pulls. `GITHUB_TOKEN` cannot do this.
- A tag push also triggers `ci.yml`'s own `push:` trigger; the duplicate run is a DESIGN item (OQ-6).
- Dependencies: US-1 (migrate), US-2 (fail-fast), both in the same feature.

### US-4 — Upgrade a demo database in place and set my own password

job_id: J13 | persona: P2 | slice: 04

Problem: Tomasz Kowalski has run the demo stack at Kestrel for six weeks; his database holds 1,240 entries and a `ledgerops_app` role with the password `app-secret`. He cannot move to a published image without either rebuilding his data or leaving a well-known password on a ledger holding real balances, because the original `IF NOT EXISTS` guard never resets the role.

#### Elevator Pitch
Before: Tomasz cannot change the app password on an existing database or move to a published release without starting over.
After: run `docker run --rm -e LEDGEROPS_MIGRATE_DSN=... -e LEDGEROPS_APP_PASSWORD_FILE=... ghcr.io/gizawnahom/ledgerops:0.2.0 migrate` → sees `applied 2 pending migrations; app role password updated; schema at version 6`
Decision enabled: Tomasz decides to restart his app on the new image and new password, knowing history is intact.

Domain Examples
1. Tomasz's database is at schema version 4 with entries across `alice-01`, `bob-01` and the demo tenant. Migrating applies versions 5 and 6; every balance and entry is unchanged and the trial balance is zero.
2. He rotates from `app-secret` to `Zr4#mV8$wQ1` on the same database; later rotates again to `Lm2&pT6@sD9`. Each run applies the password given.
3. His privileged role on a managed Postgres lacks the privilege to alter roles; migrate stops with a message naming the missing privilege and leaves the old password in place.

UAT Scenarios (BDD)

Scenario: An existing demo database is brought current without touching history
Given a demo-created database at schema version 4 with 1,240 entries and trial balance zero
When Tomasz runs `migrate` from release `0.2.0`
Then schema is at version 6
And all 1,240 entries and every balance are unchanged and the trial balance is zero

Scenario: The old well-known password stops working
Given `ledgerops_app` currently accepts `app-secret`
When Tomasz migrates with the password `Zr4#mV8$wQ1`
Then `ledgerops_app` accepts `Zr4#mV8$wQ1` and refuses `app-secret`

Scenario: Running migrate again with the same password changes nothing
Given the database is current and the password is already `Zr4#mV8$wQ1`
When Tomasz runs `migrate` again with the same password
Then it exits 0 and reports no pending migrations

Scenario: A later run with a different password rotates it
Given the password is `Zr4#mV8$wQ1`
When Tomasz runs `migrate` with `Lm2&pT6@sD9`
Then only `Lm2&pT6@sD9` is accepted afterwards

Scenario: History stays append-only after the upgrade
Given the upgrade completed
When `ledgerops_app` tries to UPDATE or DELETE an entry
Then both are refused, as before the upgrade

Scenario: A privileged role that cannot manage roles fails clearly
Given Tomasz's privileged role cannot alter other roles
When he runs `migrate` with a new password
Then it exits non-zero naming the missing privilege
And the old password still works
And the message says "no changes applied", because the privilege check runs before any schema change

Acceptance Criteria
- [ ] An existing database created by the demo stack at an earlier schema version is migrated to head with no entry, balance or trial-balance change
- [ ] An explicit supplied password is always applied, including when `ledgerops_app` already exists
- [ ] Re-running with the same inputs is a successful no-op
- [ ] The append-only guarantee (UPDATE and DELETE refused for `ledgerops_app`) holds after upgrade
- [ ] Insufficient privilege fails before any schema change, with a message naming the privilege and stating "no changes applied"; the password is unchanged
- [ ] Documentation states that a running app must be restarted after a rotation

Outcome KPIs
- Who: operators of an existing demo-created or earlier-release database
- Does what: upgrade and set their own password without data loss
- By how much: 100% of upgrade acceptance runs preserve entry count and a zero trial balance
- Measured by: acceptance suite seeded from a demo-stack database; smoke test upgrade leg
- Baseline: unsupported

Technical Notes
- Edited-`0001` safety against already-migrated databases is OQ-2. golang-migrate stores only a version number, so an edited file is not re-run on a migrated database, but this must be proven, not assumed.
- Constraint: migrations stay expand-only (`brief.md:86-91`).
- `migrate` runs a preflight privilege probe, so insufficient privilege always fails before any schema change; "whether schema changes were applied" is therefore a definite "no changes applied" in that case.
- Dependencies: US-1.

### US-5 — Open the console and see the version from the image

job_id: J11 | persona: P2 (also serves P1) | slice: 05

Problem: The README promises an operator console, but the image has no Node build stage, so a client running only the image gets a `/console` that 404s with no error (`console_static.go:45-49`). Nor can Tomasz tell from a running container which release it is.

#### Elevator Pitch
Before: the console is unreachable from a pulled image and there is no way to read the running version.
After: run `curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/console` against the running image → sees `200`; run `docker run --rm ghcr.io/gizawnahom/ledgerops:0.1.0 version` → sees `0.1.0 (commit 3f9c2ab)` (wording illustrative)
Decision enabled: Tomasz decides whether this container is the release he intends and opens the console to check the books.

Domain Examples
1. Tomasz runs the `0.1.0` image and loads `/console` in a browser; it shows the verdict page after he enters the operator key.
2. A local `docker build` without build arguments reports `dev (commit unknown)`.
3. The SPA build fails in a CI run; the image build fails rather than producing an image without a console.

UAT Scenarios (BDD)

Scenario: The console loads from the image
Given the image is running with valid configuration
When Tomasz requests `/console`
Then the console page is returned

Scenario: The console data still needs the operator key
Given the console page loaded
When it requests `/console/verdict` without a key
Then it receives 401, and with the operator key it receives the verdict

Scenario: The image reports its version
Given an image built for `0.1.0` at commit `3f9c2ab`
When Tomasz runs the `version` command
Then it prints `0.1.0` and `3f9c2ab`

Scenario: A local build is recognisably unreleased
Given an image built without version information
When Tomasz runs `version`
Then it prints `dev`

Scenario: A failed console build never yields an image
Given the console build fails
When the image is built
Then the build fails and no image is produced

Acceptance Criteria
- [ ] `/console` returns the SPA shell from the image, with no files mounted from outside
- [ ] `/console/verdict` behavior is unchanged
- [ ] `version` prints version and commit; a build with no version information prints `dev`
- [ ] An image build with a failed console build fails
- [ ] The release smoke check also asserts `/console` returns 200

Outcome KPIs
- Who: operators running the published image
- Does what: reach the console without building anything
- By how much: 100% of published images return 200 on `/console` in the smoke check
- Measured by: release smoke job
- Baseline: 0% (console not in the image)

Technical Notes
- OQ-5: the dist path is relative to the working directory today; whether to embed the bundle or keep the path convention is DESIGN's. The silent-skip probe in `mountConsole` must not be able to hide a missing bundle in a release image.
- Dependencies: none; the smoke assertion lands once US-3 exists.

### US-6 — Get the right tags, architecture and release page

job_id: J11 | persona: P1 | slice: 06

Problem: Priya works on an Apple Silicon laptop and deploys on amd64. She needs `latest` to mean "newest stable", `0.2` to track patch fixes, release candidates not to disturb either, and the image to run natively on both architectures. Without clear labels she cannot tell which commit an image came from.

#### Elevator Pitch
Before: only a single exact tag exists and only for one architecture, with no record of its commit.
After: run `docker pull ghcr.io/gizawnahom/ledgerops:latest` then `docker inspect --format '{{ index .Config.Labels "org.opencontainers.image.version" }}' ghcr.io/gizawnahom/ledgerops:latest` → sees `0.2.0` (pulled natively as arm64 on her Mac)
Decision enabled: Priya decides whether to track `latest`, `0.2` or an exact version.

Domain Examples
1. `v0.2.0` publishes `0.2.0`, `0.2` and `latest` on the same digest.
2. `v0.3.0-rc.1` publishes `0.3.0-rc.1` only; `latest` and `0.3` are untouched; the GitHub Release is marked pre-release.
3. `v0.1.9` is tagged after `0.2.0` exists: `0.1` moves to `0.1.9`, `latest` stays on `0.2.0`.

UAT Scenarios (BDD)

Scenario: A stable release gets exact, minor and latest tags
Given no earlier release exists
When `v0.2.0` is released
Then `0.2.0`, `0.2` and `latest` refer to the same image

Scenario: A prerelease never moves latest or the minor tag
Given `latest` is `0.2.0`
When `v0.3.0-rc.1` is released
Then only `0.3.0-rc.1` is added and `latest` is still `0.2.0`

Scenario: A patch moves the minor tag but keeps the old exact tag
Given `0.2` refers to `0.2.0`
When `v0.2.1` is released
Then `0.2` refers to `0.2.1` and `0.2.0` still exists unchanged

Scenario: A backport does not move latest
Given `latest` is `0.2.0`
When `v0.1.9` is released
Then `0.1` moves to `0.1.9` and `latest` is still `0.2.0`

Scenario: The image runs natively on both architectures
Given a stable release
When Priya inspects the image index
Then it lists linux/amd64 and linux/arm64

Scenario: The image says where it came from
Given release `0.2.0` built from commit `3f9c2ab`
When Priya reads the image labels
Then version is `0.2.0`, revision is `3f9c2ab` and source is the repository URL

Scenario: Each tag has a GitHub Release
Given `v0.2.0` and `v0.3.0-rc.1` are released
When Priya opens the repository's releases
Then both appear, and the rc is marked pre-release
And each release body names the image reference, its digest and the verify command, followed by auto-generated notes

Acceptance Criteria
- [ ] Stable tags publish `X.Y.Z`, `X.Y`, `latest`; prereleases publish the exact version only
- [ ] `latest` moves only when the new stable version is the highest stable version
- [ ] Older exact tags are never moved
- [ ] The image index contains linux/amd64 and linux/arm64
- [ ] OCI labels for version, revision and source are present and correct
- [ ] A GitHub Release is created for each tag, prerelease flagged correctly; its body names the image reference (`ghcr.io/gizawnahom/ledgerops:X.Y.Z`), its digest and the verify command, followed by auto-generated notes
- [ ] `latest` is moved only after the usability check passes

Outcome KPIs
- Who: integrating developers on amd64 and arm64
- Does what: pull a pinned or tracking tag and run it natively
- By how much: 100% of releases carry both architectures and correct labels
- Measured by: release workflow verification step reading the index and labels
- Baseline: none published

Technical Notes
- Taste-test note: this slice combines tags, arm64, labels and the GitHub Release (four items), all configuration of one release workflow. If it passes one day, split arm64 into its own slice 06b.
- Arm64 usability check under emulation is a DESIGN item (OQ-6).

### US-7 — Verify the image came from the project's pipeline

job_id: J12 | persona: P2 | slice: 07

Problem: Tomasz is about to run an image that holds platform funds. Anyone can push a look-alike to a registry, and a tag alone proves nothing about who built it or what is inside.

#### Elevator Pitch
Before: Tomasz can only trust the tag name.
After: run `cosign verify ghcr.io/gizawnahom/ledgerops:0.2.0 --certificate-identity-regexp '^https://github.com/gizawnahom/ledgerops/' --certificate-oidc-issuer https://token.actions.githubusercontent.com` → sees `The cosign claims were validated` and the signing identity (wording illustrative)
Decision enabled: Tomasz decides whether this image may be deployed.

Domain Examples
1. Tomasz verifies `0.2.0`; it passes and names the release workflow at tag `v0.2.0`.
2. Someone pushes `ghcr.io/gizawnahom/ledgerops:0.2.0` look-alike from another account; verification fails.
3. Tomasz downloads the SBOM attestation of `0.2.0` and finds the Go module list, including `pgx`.

UAT Scenarios (BDD)

Scenario: A released image verifies against the project's identity
Given release `0.2.0` was published by the pipeline
When Tomasz runs `cosign verify` with the project's identity
Then verification succeeds

Scenario: A look-alike image fails verification
Given an image with the same name built elsewhere
When Tomasz runs the same command
Then verification fails

Scenario: The software bill of materials can be retrieved for a platform
Given release `0.2.0`
When Tomasz resolves the digest for his platform and fetches that digest's SBOM attestation
Then it lists the Go modules and base-image packages for that platform
And the index signature still verifies on the tag itself

Scenario: Prereleases are signed as well
Given `0.3.0-rc.1`
When Tomasz verifies it
Then verification succeeds

Scenario: A signing failure fails the release
Given the signing step fails
When the pipeline finishes
Then the run is red and `latest` has not moved

Acceptance Criteria
- [ ] Every published tag is signed keylessly and verifies against the repository's release workflow identity
- [ ] An image not built by that workflow fails verification
- [ ] An SBOM attestation is published for every release, per platform digest (it is verified through the platform digest, not the tag)
- [ ] A signing or attestation failure fails the release and `latest` does not move
- [ ] Both architectures are covered: the index and each platform manifest are signed, and each platform carries its own SBOM attestation

Outcome KPIs
- Who: operators about to deploy the image
- Does what: verify provenance before running
- By how much: 100% of stable releases verify and carry an SBOM
- Measured by: final release step runs `cosign verify` and, for each published platform digest, `verify-attestation`
- Baseline: none

Technical Notes
- Signing covers the index and each platform manifest; SBOM is per platform (settled in DESIGN, ADR-021).
- The workflow needs the OIDC token permission (OQ-6).
- Dependencies: US-3, US-6.

### US-8 — Follow a quickstart that uses the published image

job_id: J11 | persona: P1 | slice: 08

Problem: Priya has a pullable image but no documentation for it. The README quickstart still builds from source, its limitations list says "no published container image; you build from source", and nothing explains the migrate step, the password, or the keys.

#### Elevator Pitch
Before: the only documented path is `git clone` and `make up`.
After: follow `docs/quickstart-published-image.md` from `docker pull ghcr.io/gizawnahom/ledgerops:0.1.0` → sees a first transfer's JSON response with a transaction id
Decision enabled: Priya decides to wire her service to this deployment pattern.

Domain Examples
1. Priya, with only Docker, runs the documented commands and posts a transfer between `alice` and `bob` without cloning anything.
2. Tomasz follows the upgrade section to move the Kestrel demo database to `0.2.0` and rotate the password.
3. Priya copies the verify command and runs it against the pinned tag.

UAT Scenarios (BDD)

Scenario: The documented commands work as written
Given a machine with Docker and no source checkout
When Priya runs the quickstart's commands in order against the latest release
Then a transfer is posted and the response contains a transaction id

Scenario: The README no longer says to build from source
Given the README
When Priya reads Quickstart and Known limitations
Then the published image is the primary path and the "no published container image" limitation is gone

Scenario: Secrets are treated as secrets
Given the quickstart
When Priya reads its configuration steps
Then it generates the app password and keys rather than reusing demo values, and warns against `app-secret` and `demo-*` keys

Scenario: The upgrade path is documented
Given Tomasz's demo-created database
When he reads the upgrade section
Then it tells him to run `migrate` with his own password first and then restart the app, in that order

Scenario: The verification command is documented with the real identity
Given the quickstart
When Priya copies its verify command
Then it uses the documented release-workflow identity pattern and succeeds against a release

Scenario: The docs cover the traps a careful operator would hit
Given the quickstart
When Tomasz reads the secrets, recovery and constraints sections
Then they steer to the password file, warn that DDL statement logging exposes the password verifier, explain manual dirty-migration recovery, and state one ledgerops database per Postgres cluster

Scenario: The docs recommend pinning by digest
Given the quickstart
When Priya reads how to reference the image
Then it recommends pinning by digest and explains the registry cannot enforce tag immutability

Acceptance Criteria
- [ ] The quickstart's commands run verbatim against the published image in CI
- [ ] README Quickstart leads with the published image; the limitation line is removed or reworded; roadmap item "Adoptable" is amended to say what remains (OpenAPI, transfer metadata)
- [ ] No demo credential is used in the published-image path
- [ ] Upgrade, rotation and verification sections exist; the verify command uses the identity regex from ADR-021
- [ ] The document states the pin-by-digest recommendation (the registry cannot enforce tag immutability)
- [ ] The rotation instructions state the order: run `migrate`, then restart the app (ADR-023)
- [ ] The verification section shows both commands: `cosign verify` on the tag (index signature), and platform-digest lookup followed by `verify-attestation` for the SBOM
- [ ] Secrets guidance steers to `LEDGEROPS_APP_PASSWORD_FILE` (environment variables are visible through `docker inspect`) and states that under `log_statement=ddl` the password verifier is logged and is offline-attackable, so a generated high-entropy password is required
- [ ] Manual dirty-migration recovery is documented: check whether migration N's objects exist, set `schema_migrations` to N-1 or N with `dirty = false`, then re-run `migrate`
- [ ] The one-ledgerops-database-per-cluster constraint and its remedy (separate cluster, or migrate the database that owns the role) are stated, including `DROP ROLE ledgerops_app` as a privileged role for a dropped owning database or an abandoned first run
- [ ] A compose or `docker run` example for the published image exists separately from `docker-compose.yml`

Outcome KPIs
- Who: integrating developers
- Does what: reach a first posted transfer using only the docs and Docker
- By how much: under 10 minutes from `docker pull` to first transfer, with 100% of documented commands passing in CI
- Measured by: CI run of the quickstart commands; timing recorded
- Baseline: a source checkout and Go/Docker build

Technical Notes
- Exact file locations are DESIGN/DELIVER's; README and docs edits are ordinary files, not wave artifacts.
- Dependencies: US-3 through US-7 (needs a real tag and signing identity).

---

## Wave: DISCUSS / [REF] Outcome KPIs

| # | Who | Does what | By how much | Baseline | Measured by | Type |
|---|---|---|---|---|---|---|
| K1 | Integrating developers | Reach a healthy service from `docker pull` | at most 120 s in the smoke check; at most 10 min to first transfer by quickstart | impossible without source | release smoke and quickstart CI | Leading |
| K2 | Release consumers | Get only images that migrate and serve | 100% of releases pass smoke before `latest` moves | none published | release workflow | Leading |
| K3 | Anyone running the image | Cannot reach the database with a password they did not supply | 100% refusal of `app-secret` in fresh-database acceptance | 0% | acceptance and smoke | Guardrail |
| K4 | Operators of demo-created databases | Upgrade with no data change | 100% preserved entries and zero trial balance | unsupported | upgrade acceptance | Leading |
| K5 | Operators | Verify provenance | 100% of stable releases pass `cosign verify` and carry an SBOM | none | final release step | Leading |
| K6 | Operators | Reach the console from the image | 100% of images return 200 on `/console` | 0% | smoke | Leading |
| K7 | Apple Silicon and amd64 users | Pull natively | 100% of releases list both architectures | none | release verification step | Leading |

North star: K1. Guardrails: the demo path stays green (`make demo-01..04`, `chaos-01`, `race-02/03`, all CI jobs); the append-only guarantee still holds after upgrade. Measurement is all inside CI; no production telemetry exists (`brief.md:67-71`). KPI contract wiring in `docs/product/kpi-contracts.yaml` is DEVOPS's.

Hypothesis: we believe that a gated, smoke-tested, signed multi-arch image with its own migrate command will let an integrating developer run a pinned ledgerops without a source checkout. We will know it is true when a developer with only Docker reaches a posted transfer in under 10 minutes using the documented commands.

## Wave: DISCUSS / [REF] Definition of Done (DoD)

1. All UAT scenarios across US-1 to US-8 pass (green)
2. Supporting tests pass (unit for config validation; integration for `migrate` and password handling against Postgres; workflow checks)
3. Code refactored, reviewed and approved
4. Merged to main
5. Demoable: from a clean machine with Docker, pull the latest rc, migrate, start and post a transfer
6. A real rc tag (`v0.1.0-rc.1`) has been published, smoke-passed and, once slice 07 lands, verified. The first release run is expected to fail at the anonymous-pull check until the owner sets the GHCR package public (OQ-7); the owner then re-runs
7. Existing `demo-*`, `chaos-*`, `race-*` targets and all CI jobs pass unmodified
8. D1-D9 are honored or any change is recorded with the user's approval (D9 revised 2026-10-17); OQ-1, OQ-11 and OQ-12 closed
9. README and quickstart reflect the published path

## Wave: DISCUSS / [REF] Out-of-scope

- Hosted deployment, Kubernetes manifests or Helm charts (no hosted environment exists, OPS-1)
- Automatic migration at app start (explicitly rejected by D4)
- Docker Hub or any second registry (D1)
- OpenAPI spec, client SDKs, transfer metadata (rest of the "Adoptable" roadmap item)
- TLS in the service, secrets management integrations beyond env and file
- Tenant key rotation (existing known limitation)
- Changing the domain core or any API behavior
- Automated changelog tooling (release notes are auto-generated by GitHub, OQ-12)

## Wave: DISCUSS / [REF] WS strategy

A: thin, sequential slices with no environment switch. Walking skeleton is slices 01-03. The password, keys and DSNs are ordinary configuration, not an environment-switching mechanism.

## Wave: DISCUSS / [REF] Driving ports

| Port | Surface | Slice |
|---|---|---|
| `ledgerops migrate` with `LEDGEROPS_MIGRATE_DSN` (name illustrative), `LEDGEROPS_APP_PASSWORD[_FILE]` | container command and env | 01, 04 |
| Default container start with `LEDGEROPS_APP_DSN`, `LEDGEROPS_OPERATOR_KEY` (required) and `LEDGEROPS_DEMO_TENANT_KEY` (optional) | container command and env | 02 |
| `git push` of a `v*.*.*` tag | GitHub Actions trigger | 03, 06 |
| `docker pull ghcr.io/gizawnahom/ledgerops:<tag>` | registry | 03, 06 |
| `ledgerops version` | container command | 05 |
| `GET /health`, `GET /console`, `GET /console/verdict` | HTTP (existing) | 03, 05 |
| `cosign verify`, `cosign verify-attestation` | registry artifacts | 07 |

All command and variable names are illustrative except those fixed by D4 and D5.

## Wave: DISCUSS / [REF] Pre-requisites (for DESIGN)

Open questions handed to DESIGN. Closed by the user on 2026-10-17: OQ-1 (demo key optional), OQ-11 (keep one feature), OQ-12 (v0.1.0, auto-generated notes). OQ-7 remains an owner action.

1. OQ-1 CLOSED: `LEDGEROPS_DEMO_TENANT_KEY` is optional; see D9 as revised.
2. OQ-2 How to change `0001` (hardcoded `GRANT CONNECT ON DATABASE ledgerops` and the `app-secret` role creation) safely: edit the applied file (golang-migrate does not checksum, but a new database with another name needs the edit) versus a new migration. Includes closing the window between `0001` creating the role and the Go-side password being set, and the crash-in-between case.
3. OQ-3 Entrypoint shape: today `ledgerops-api`; D4 names `ledgerops migrate`. A multi-command `ledgerops` binary with no-argument = serve keeps compose working. Exact env var for the privileged DSN. Flag for both-variables-set (`PASSWORD` and `PASSWORD_FILE`): recommend refuse as ambiguous.
4. OQ-5 Console delivery: working-directory-relative `web/console/dist` versus embedding; silent probe-guard in `mountConsole` must not hide a missing bundle in a release image.
5. OQ-6 Release workflow: refactor `ci.yml` for `workflow_call`; a tag push also triggers `ci.yml`'s own `push:`, producing a duplicate run; order of push-by-digest, smoke, promote tags, sign, release (`latest` moves last); arm64 usability under emulation; required permissions (`packages: write`, `id-token: write`, attestations).
6. OQ-7 GHCR package visibility: first push is private; the owner must set it public and link it to the repository once. Pre-requisite for anonymous pull and for US-3's first scenario.
7. OQ-8 Privileged role requirements on bring-your-own and managed Postgres (create and alter roles; the code does not require the name `ledgerops_migrate`).
8. OQ-9 Rotation effect on a running app: pooled connections keep the old credential until restart; document, or have the probe reconnect.
9. OQ-10 SBOM format and what is signed or attested (index versus per-platform digests).
10. OQ-11 CLOSED: kept as one feature (see Scope Assessment).
11. OQ-12 CLOSED: first version `v0.1.0`, GitHub auto-generated release notes.

Dependencies on prior features: `ledger-core`, `multitenancy`, `inter-tenant-transfer`, `ledger-core-console` are shipped; nothing blocks this feature.

## Wave: DISCUSS / [REF] Requirements Completeness

Functional: covered, US-1 to US-8, 5 to 7 scenarios each. Non-functional: security (password and key handling, no secrets in logs, provenance), reliability (idempotent migrate, smoke before `latest`, immutable versions), performance (smoke at most 120 s; image size recorded, not gated), compatibility (demo path unchanged), operability (clear config errors). Business rules: tag classification and `latest` semantics explicit in US-6. Estimated completeness score: 0.96. The remaining gap is the eleven open items above, each named and owned rather than assumed.

## Wave: DISCUSS / [REF] Definition of Ready (DoR) Validation

| DoR item | Status | Evidence |
|---|---|---|
| 1. Problem statement clear, domain language | PASS | Each story opens with a named person and a concrete pain with file refs where relevant |
| 2. Persona with specific characteristics | PASS for P2 (real-project-owner); PARTIAL for P1 (archetype-unvalidated, inherited from `integrating-developer.yaml`) | Priya Nair and Tomasz Kowalski used consistently |
| 3. 3+ domain examples with real data | PASS | Every story has 3, with real names, tags, passwords and counts (1,240 entries; v0.2.0) |
| 4. UAT in Given/When/Then, 3 to 7 scenarios | PASS | US-1: 6, US-2: 5, US-3: 6, US-4: 6, US-5: 5, US-6: 7, US-7: 5, US-8: 5 |
| 5. AC derived from UAT | PASS | Each AC list maps to its scenarios |
| 6. Right-sized, 1 to 3 days | PASS per story (each slice at most 1 day); feature-level scope is oversized by heuristic, user-approved to keep as one feature (OQ-11 closed) |
| 7. Technical notes: constraints and dependencies | PASS | Per-story notes, Pre-requisites OQ-1 to OQ-12 |
| 8. Dependencies resolved or tracked | PASS with one owner action | Shipped features resolved; GHCR package visibility (OQ-7) tracked as an owner action before US-3 can be fully verified |
| 9. Outcome KPIs with measurable targets | PASS | Per-story KPIs and K1 to K7 |

DoR status: PASSED for all eight stories. Re-run 2026-10-17 after item 14 (US-1, US-8): US-1 now has 10 scenarios (exception, at the proposed split threshold; see Applied note), US-8 has 7 (within range); other DoR items pass. Earlier: re-run after review-cycle-1 items 9-13 (US-1, 3, 6, 7, 8): items 1-3, 5, 7-9 still PASS; scenario counts now US-1 9 (exception, same reasoning as below), US-3 6, US-6 7, US-7 5, US-8 7 (within 3-7). Re-run 2026-10-17 after the DESIGN back-propagation edits (US-1, 2, 3, 4, 7, 8): items 1-3, 5, 7-9 still PASS. Item 4/6 scenario count is now over the 3-7 guideline for two stories: US-1 has 8 and US-2 has 9 (US-3 6, US-4 6, US-7 5, US-8 6 are within). The extra scenarios are variants of the migrate-safety rule (US-1) and the startup-config rule (US-2); slice effort is unchanged at one day or less. PO accepts this as a documented exception, to be revisited by DISTILL, which may split a story's scenarios across two acceptance features without changing slices. Earlier amendment note follows. Re-run 2026-10-17 after the amendments: still PASSED. US-2 now has 8 scenarios (within the 3 to 7 guideline by one; the three demo-key scenarios are short variants of one rule, so the story is not split, noted for DESIGN). Carried: P1 archetype caveat (inherited), OQ-7 owner action. OQ-1, OQ-11, OQ-12 closed. Peer review (`nw-product-owner-reviewer`) was not run in this dispatch; it is optional per the DISCUSS skill and the mandatory consolidated review fires at the end of DISTILL.

## Wave: DISCUSS / [REF] Wave Decisions Summary

# DISCUSS Decisions — container-image-release

## Key Decisions
- [D1-D9] User answers as listed in Locked decisions; D9 revised 2026-10-17 (demo tenant key optional)
- [D10] Scope oversized by heuristic, user-approved to keep as one feature with 8 slices of at most one day (OQ-11 closed)
- [D14] First version v0.1.0, GitHub auto-generated release notes (OQ-12 closed)
- [D11] Walking skeleton is slices 01-03, ordered by uncertainty
- [D12] Added J12 (provenance) and J13 (upgrade in place) beside J11
- [D13] New journey `pull-and-run-ledgerops`

## Requirements Summary
- Primary jobs: J11 (pull and run a pinned image), J12 (verify provenance), J13 (upgrade in place with own password)
- Walking skeleton: migrate with own password, refuse bad config, gated publish with smoke
- Feature type: cross-cutting (release CI, packaging, app configuration)

## Constraints Established
- App never holds the migrate role or privileged DSN (D4, `main.go` header)
- Migrations stay expand-only and entries stay append-only
- No password, key or DSN value appears in any log or output
- `latest` moves last and only for the highest stable version
- Published versions are immutable
- Demo path (`make up`, demo-*, chaos-*, race-*) behaves exactly as before

## Upstream Changes
- `docs/product/jobs.yaml`: J11, J12, J13 added (founder-estimated)
- `docs/product/personas/integrating-developer.yaml` jobs += J11; `platform-operator.yaml` jobs += J12, J13
- `docs/product/journeys/pull-and-run-ledgerops.yaml` created
- README limitation and roadmap lines to be amended in slice 08 (not a DISCOVER assumption)

## Wave: DISCUSS / [REF] Next Wave

Handoff to `nw-solution-architect` (DESIGN, full artifact set) and `nw-platform-architect` (DEVOPS, KPI sections only). Deliverables: this file, `slices/slice-01..08-*.md`, and the SSOT updates above. Peer review not yet run. No user answers outstanding; OQ-7 is an owner action.

---

## Wave: DISCUSS / [HOW] Gherkin scenarios

Rendered on request (expansion `gherkin-scenarios`, triggered by AC ambiguity). Declined by the user: `alternatives-considered`, `migration-playbook`.

The ambiguity: US-1 and US-4 both say "the app role never accepts a password the operator did not supply". Readers could disagree about two moments. The rule below settles them, stated as observable behavior; how it is achieved (OQ-2) is DESIGN's.

Rule A (the window during migrate): from the moment a fresh `migrate` run starts until it finishes or dies, a login as `ledgerops_app` is accepted only with the operator's password or not at all. `app-secret` is never accepted on a database this run created, at any instant, including after a crash.
Rule B (the pre-rotation existing database): a database created before this feature already accepts `app-secret`. That is existing state, not a violation. It stays so until a `migrate` run with an explicit password completes the password step; if that run fails or is interrupted before the password step, `app-secret` still works and the output says so. Rule A applies only to databases the run creates.

```gherkin
Feature: Container image release

  # --- US-1 / Rule A: the window during migrate on a fresh database
  Scenario: A fresh database never accepts the demo password at any point during migrate
    Given Priya has an empty database "kestrel_ledger" and supplies the password "T7!kq@9x%Zm"
    When migrate runs and is observed after every schema step
    Then a login as "ledgerops_app" with "app-secret" is refused at every observation
    And a login with "T7!kq@9x%Zm" succeeds once migrate reports success

  Scenario: A crash partway through a fresh migrate leaves no demo password behind
    Given Priya has an empty database "kestrel_ledger" and supplies "T7!kq@9x%Zm"
    And migrate is killed between two migration files
    When someone logs in as "ledgerops_app" with "app-secret"
    Then the login is refused
    And re-running migrate with "T7!kq@9x%Zm" completes and that password works

  Scenario: A crash during a migration file leaves a dirty version that migrate refuses to continue from
    Given Priya has an empty database "kestrel_ledger" and supplies "T7!kq@9x%Zm"
    And migrate is killed during a migration file
    When she re-runs migrate
    Then it exits non-zero naming the dirty version and changes nothing
    And a login as "ledgerops_app" with "app-secret" is refused

  Scenario: Migrate refuses with no password before touching the database
    Given Priya supplies neither LEDGEROPS_APP_PASSWORD nor LEDGEROPS_APP_PASSWORD_FILE
    When she runs migrate against an empty database
    Then it exits non-zero naming both variables
    And no table and no "ledgerops_app" role exist afterwards

  Scenario: Both password sources set is refused as ambiguous
    Given Priya sets LEDGEROPS_APP_PASSWORD and LEDGEROPS_APP_PASSWORD_FILE to different values
    When she runs migrate
    Then it exits non-zero explaining that only one may be set
    And the database is unchanged

  Scenario: A password file with a trailing newline yields the password without it
    Given Tomasz's secret file contains "Zr4#mV8$wQ1" and a newline
    When he runs migrate
    Then "ledgerops_app" logs in with "Zr4#mV8$wQ1"

  Scenario: Migrate output never contains the password
    Given Priya supplies "T7!kq@9x%Zm"
    When migrate succeeds or fails for any reason
    Then neither standard output nor standard error contains "T7!kq@9x%Zm"

  # --- US-4 / Rule B: the pre-rotation existing database
  Scenario: An existing demo database keeps accepting the old password until the password step completes
    Given a demo-created database at schema version 4 whose "ledgerops_app" accepts "app-secret"
    When migrate with "Zr4#mV8$wQ1" fails during a schema step
    Then "app-secret" still works
    And the output states the password was not changed

  Scenario: A completed upgrade replaces the old password
    Given the same database
    When migrate with "Zr4#mV8$wQ1" completes
    Then "Zr4#mV8$wQ1" is accepted and "app-secret" is refused
    And all 1,240 entries and every balance are unchanged and the trial balance is zero

  Scenario: Rotating again applies the newest password
    Given the password is "Zr4#mV8$wQ1"
    When Tomasz runs migrate with "Lm2&pT6@sD9"
    Then only "Lm2&pT6@sD9" is accepted

  Scenario: Re-running with identical inputs is a successful no-op
    Given the database is current with password "Zr4#mV8$wQ1"
    When migrate runs again with "Zr4#mV8$wQ1"
    Then it exits 0 and reports no pending migrations

  Scenario: A privileged role that cannot manage roles leaves everything as it was
    Given the privileged role cannot alter other roles
    When Tomasz runs migrate with a new password
    Then it exits non-zero naming the missing privilege
    And the old password still works
    And the output says "no changes applied"

  Scenario: History stays append-only after upgrade
    Given the upgrade completed
    When "ledgerops_app" tries to UPDATE or DELETE an entry
    Then both are refused

  # --- US-2: startup configuration
  Scenario: Missing required settings are all named in one refusal
    Given LEDGEROPS_OPERATOR_KEY and LEDGEROPS_APP_DSN are unset
    When the container starts
    Then it exits 1 naming both and has made no database connection

  Scenario: The demo tenant key is optional and off when unset
    Given the DSN and operator key are set and LEDGEROPS_DEMO_TENANT_KEY is unset
    When a caller presents the token "demo-tenant-key"
    Then the container is serving and the token is refused 401

  Scenario: The serving process refuses to start holding the migration credential
    Given LEDGEROPS_MIGRATE_DSN is present in the serve environment
    When the container starts to serve
    Then it exits 1 naming LEDGEROPS_MIGRATE_DSN with no database connection made

  Scenario: An unreadable password file gives a named error
    Given the password file is mode 0400 owned by root and the image runs non-root
    When migrate runs
    Then it exits non-zero naming the file and the database is unchanged

  Scenario: Wrong app password at start is a clear refusal
    Given the database was migrated with "T7!kq@9x%Zm" and the DSN carries "wrong-guess"
    When the container starts
    Then it exits non-zero naming the role and that authentication failed
    And no log line contains either password

  # --- US-3 / US-6: publishing
  Scenario: A strict version tag publishes a pullable, smoke-tested image
    Given main is green at the commit tagged "v0.1.0"
    When the release pipeline finishes
    Then "ghcr.io/gizawnahom/ledgerops:0.1.0" pulls without login
    And the smoke check ran against the pushed digest before "0.1.0" was tagged, migrated a fresh database and saw /health 200

  Scenario: A red build publishes nothing
    Given an integration job fails at the commit tagged "v0.1.1"
    When the pipeline runs
    Then no image exists for "0.1.1"

  Scenario: Non-version tags and branches publish nothing
    Given the tags "v1.2" and "nightly-2026-10-17" and a branch push
    When the pipelines run
    Then no image is published

  Scenario: A moved version tag never overwrites a published version
    Given "0.1.0" is published
    When "v0.1.0" is force-moved and pushed
    Then the pipeline refuses and the digest of "0.1.0" is unchanged

  Scenario: A prerelease never moves latest or the minor tag
    Given "latest" is "0.2.0"
    When "v0.3.0-rc.1" is released
    Then only "0.3.0-rc.1" is added and "latest" is still "0.2.0"

  Scenario: A backport moves its minor tag but not latest
    Given "latest" is "0.2.0"
    When "v0.1.9" is released
    Then "0.1" refers to "0.1.9" and "latest" is still "0.2.0"

  # --- US-5 / US-7: console, version, provenance
  Scenario: The image serves the console and reports its version
    Given the image for "0.1.0" at commit "3f9c2ab" is running
    When Tomasz requests /console and runs the version command
    Then /console returns 200 and version prints "0.1.0" and "3f9c2ab"

  Scenario: A failed console build yields no image
    Given the console build fails
    When the image is built
    Then the build fails

  Scenario: A released image verifies and a look-alike does not
    Given release "0.1.0" and a look-alike under the same name
    When Tomasz runs cosign verify with the project identity on each
    Then only the release verifies
    And the release's SBOM attestation lists its Go modules
```

---

## Wave: DESIGN / [REF] Prior Wave Consultation

Scope: application (nw-solution-architect), mode: propose, user accepted every recommendation 2026-10-17. Density: lean (Tier-1 only).

- ✓ `docs/product/architecture/brief.md` (read in parts; extended, not recreated), ADR-007 and ADR-013 in full; ADRs 001-006, 008-012, 014-016 not read in full (none touches packaging)
- ✓ `docs/product/journeys/pull-and-run-ledgerops.yaml`
- ✓ This file's DISCUSS sections; ⊘ `slices/*.md` (globbed, content duplicated here)
- ✓ `CLAUDE.md` (functional paradigm, pure core / effect shell; not re-asked)
- ✓ Codebase: `Dockerfile`, `docker-compose.yml`, `ci.yml`, `cmd/api/main.go`, `postgres/store.go`, `0001_init.up.sql`, `console_static.go`, `web/console/package.json`; by grep: `Makefile`, workflow triggers, `router.go`, `.golangci.yml`, migration grants
- ⊘ `discover/`, `diverge/`, `design/`, `spike/` (not found)

No unresolved contradictions with DISCUSS. Five refinements are recorded under § Changed Assumptions. Revised in review cycle 1 (2026-10-17): DDD-27..40 amended where noted, DDD-41..47 added; the PO's applied upstream changes 1-8 were re-read and are consistent with this wave.

## Wave: DESIGN / [REF] DDD list

Numbering continues from DDD-26 (`multitenancy`/`inter-tenant-transfer`). ADRs 017-023.

| ID | Decision | One-line rationale |
|---|---|---|
| DDD-27 | Edit applied migration 0001 once, in place: `ledgerops_app` created NOLOGIN, no password; `GRANT CONNECT` via `current_database()` (ADR-017) | A new migration cannot help; 0001 itself fails on any other database name; golang-migrate never re-runs it |
| DDD-28 | Password step is Go-side, last, always applied, sent as a client-built SCRAM-SHA-256 verifier via a password-only capability (ADR-017, ADR-022) | Rule A by construction (no login at any instant); plaintext never reaches server logs or a non-TLS wire |
| DDD-29 | Dirty migration state: refuse naming the version and both candidate states, no `--force` in v0.1.0, documented manual recovery, premise guarded by a kill-mid-file test (ADR-017) | Verified in golang-migrate 4.18.3: each file is one implicit transaction, so the schema is at N-1 or N and the version row cannot say which; forcing could corrupt a ledger |
| DDD-30 | One binary `ledgerops`: `serve` (default), `migrate`, `version`; directory stays `cmd/api` (ADR-018) | D4; compose keeps working; no churn in documents that cite `cmd/api` |
| DDD-31 | Variables `LEDGEROPS_MIGRATE_DSN`, `LEDGEROPS_APP_PASSWORD[_FILE]`; both password sources set refuses; blank = unset; one trailing newline stripped; `serve` refuses when `LEDGEROPS_MIGRATE_DSN` is present; config parsing is a pure function (ADR-018) | Fail fast, list every problem, protect OPS-10 against shared env files |
| DDD-32 | `Secret` redacting type, DSN error redaction, wrong app password at start classified (SQLSTATE 28P01) into `health.startup.refused` (ADR-018) | AC: no secret in any output; named refusal instead of a first-request failure |
| DDD-33 | Console embedded via a `console_embed` build-tagged file pair in the existing HTTP adapter behind an `fs.FS` seam; every image build passes the tag; startup refusal if embedded `index.html` is missing (ADR-019) | Closes the silent-404 bug class; dev/CI builds unchanged; no new package |
| DDD-34 | Image build shape: one Dockerfile, Node stage, `$BUILDPLATFORM` cross-compile, ldflags stamping, alpine with no `RUN` in the final stage, numeric non-root `USER 65532:65532`, OCI labels, `.dockerignore` (ADR-019) | What is tested is what ships; no QEMU for compile; non-root for a ledger holding funds |
| DDD-35 | `release.yml` on version-tag push; `ci.yml` gains `workflow_call`, `push:` becomes branches-only; digest-first order (build, smoke, sign/attest/verify, anonymous-pull probe, then tags promoted last) (ADR-020) | No duplicate CI run; no public version tag before smoke, signature and public visibility |
| DDD-36 | `internal/release` pure classifier produces a `ReleasePlan`; workflow only executes it (ADR-020) | Backport/prerelease/`latest` rules are testable business logic, not inline shell |
| DDD-37 | Tag immutability guard (absent or same digest), global non-cancelling concurrency group, least-privilege per-job permissions (sign job: `packages: write` + `id-token: write`; smoke: `packages: read`), frozen `release.yml` identity (ADR-020) | Re-runs are idempotent; "highest stable" is evaluated serially; the signing identity includes the workflow path |
| DDD-38 | Keyless cosign `--recursive` over index and platforms (verify works on the tag); per-platform SPDX-JSON SBOM via syft attached with `cosign attest --type spdxjson` (ADR-021) | Matches `cosign verify`/`verify-attestation` stories; SBOM content is per platform |
| DDD-39 | Supported minimum privilege set for the migrate role as an explicit checklist (database CONNECT-grant right, CREATE on `public`, CREATEROLE, PG16 ADMIN OPTION, ownership of altered tables, cluster and collision rules), gathered as facts plus always-rolled-back empirical probes, mapped 42501 backstop (ADR-022) | Failure before any change, naming the privilege; PG16 `CREATEROLE` semantics |
| DDD-40 | Rotation is migrate-then-restart; hot reload recorded, not built (ADR-023) | Pooled connections keep the old credential only until recycled; no story asks for hot reload |
| DDD-41 | One ledgerops database per Postgres cluster (user decision): `migrate` against a target with no ledgerops schema refuses, before any change, when role `ledgerops_app` already exists in the cluster; no override flag; atomic only through the role claim of DDD-48 (ADR-017) | The role is cluster-global: Rule A holds by construction and a rotation can never reach another database |
| DDD-42 | Migration is a pure `PlanMigration(facts, config)` returning a refusal or an ordered plan; facts come from read-only catalog queries plus rolled-back empirical probes; driven ports split into `SchemaMigrator` and `RolePasswordSetter` (takes a finished verifier only) (ADR-022) | Every refusal reason is table-testable; the password setter cannot touch schema and never sees plaintext |
| DDD-43 | SBOM verify target is the platform digest (two-step lookup documented); no index-level SBOM; cosign 2.x, digest references only (ADR-021) | One merged SBOM would be wrong for one platform; avoids relying on an unverified `--platform` flag |
| DDD-44 | Pinning policy: every `uses:` by full commit SHA, tool versions pinned, all base images (incl. smoke `postgres:16`) by digest, automated unpinned-`uses:` check (ADR-019, ADR-020) | Supply-chain integrity of the pipeline that signs the image |
| DDD-45 | Re-run semantics: if the exact tag exists, resume with its digest (skip build), else build anew; builds not claimed reproducible; no automatic pruning in v0.1.0 (ADR-020) | Idempotent re-runs without reproducible-build effort; naive pruning would break multi-arch indexes |
| DDD-46 | Smoke harness, quickstart checker and release planner are Go programs under `scripts/<name>/` (existing `scripts/race` convention); GitHub Release body is a template naming the image reference and verify commands plus generated notes; docs and example compose are components (ADR-020) | One convention; the quickstart document is the tested artifact |
| DDD-47 | SCRAM verifier uses SASLprep (RFC 4013) normalisation, injected salt and iteration count, non-ASCII acceptance case (ADR-017) | Correct for non-ASCII passwords; deterministic unit tests |
| DDD-48 | The DDD-41 guarantee is made atomic by claiming the role: when absent, `migrate` first creates `ledgerops_app` NOLOGIN with an owner-database marker (`COMMENT ON ROLE`) in its own committed transaction; the loser of a race gets 42710, re-reads the marker and refuses with the cluster-rule message; a re-run against the marked database resumes. No advisory lock (ADR-017) | `pg_advisory_lock` keys are per-database, so migrates on different databases would not contend, and a fixed shared database may deny CONNECT; role creation is the one cluster-global atomic operation. Refusal message names the privileged remedy `DROP ROLE ledgerops_app` when the marked database no longer exists |

## Wave: DESIGN / [REF] Changed Assumptions

Refinements accepted by the user 2026-10-17. DISCUSS sections are not edited; story/AC wording that needs PO revision is listed in `design/upstream-changes.md`.

1. **Smoke ordering (digest-first).** Original (this file, D8): "then a post-push smoke test pulls the published image, migrates a fresh Postgres and checks `/health`." New: the image is pushed by digest with no version tag; the smoke pulls that exact digest from GHCR, both architectures, plus the upgrade leg; signing follows; only then is `X.Y.Z` promoted, then `X.Y` and `latest`. Rationale: a failed smoke or signature leaves an untagged digest, never a public version tag. Stricter than D8, same registry bytes.
2. **Rule A crash scenario.** Original (Gherkin scenarios): "And migrate is killed after the first schema change ... And re-running migrate with "T7!kq@9x%Zm" completes and that password works". New: true only when the kill falls between migration files. A kill mid-file leaves a dirty version; `migrate` refuses naming it (DDD-29). The role stays NOLOGIN, so Rule A holds in both cases. DISTILL needs both scenarios.
3. **Test churn scope.** Original (D6): "`docker-compose.yml`, the loadtest compose, the test helper and CI set `LEDGEROPS_APP_PASSWORD=app-secret` explicitly". New: `postgres.Migrate`/`MigrateStep` change signature (password parameter) and have 7 direct call sites in 6 files (verified by grep, corrected in review cycle 1): `tests/acceptance/ledgercore/ledger_seeding.go` (2: `Migrate`, `MigrateStep`), `ledger_observations.go`, `tests/acceptance/multitenancy/world.go`, `tests/acceptance/intertenanttransfer/world.go`, `internal/adapters/postgres/testhelper_test.go`, `internal/adapters/http/trial_balance_tenant_scope_test.go`; `steps_ledger_test.go` reaches it through `MigrateFromZero`. There are at least six independent `app-secret` literals. All pass `app-secret` explicitly; the compiler finds every site.
4. **Frozen pre-feature fixture.** New obligation: before 0001 is edited, the current 0001 text (and 0002-0006) is frozen as a test fixture so US-4 and the smoke upgrade leg can build a database exactly as the pre-feature demo stack did (role LOGIN with `app-secret`, grant on database `ledgerops`).
5. **One ledgerops database per cluster (user decision, review cycle 1).** New constraint not present in DISCUSS: `ledgerops_app` is cluster-global, so a fresh `migrate` refuses when the role already exists in the cluster (DDD-41, ADR-017). DISCUSS's "A database with any name can be prepared" (US-1) stays true for the first ledgerops database in a cluster; see `design/upstream-changes.md` item 10 for the story text that needs a qualifier.

## Wave: DESIGN / [REF] Component decomposition

| Component | Path | Change | Notes |
|---|---|---|---|
| Subcommand dispatcher, `LoadServeConfig`, `ParseMigrateConfig`, startup refusal classifier, `refuseIfMigrateDSNPresent`, version vars | `cmd/api/` | EXTEND | Binary renamed `ledgerops`; directory unchanged |
| `PlanMigration` (pure), `FactReader` (read-only), `PreflightProber` (write-capable, rolled back, never committed), role claim, dirty detection, DSN redactor | `internal/adapters/postgres/` (plan and redactor may live in a pure sub-file) | EXTEND | Plan is pure over facts; shell executes |
| `SchemaMigrator` and `RolePasswordSetter` driven ports; SCRAM verifier builder | `internal/adapters/postgres/` | EXTEND | `Migrate`/`MigrateStep` become the orchestration entry points and gain a `Secret` password; the setter takes a finished verifier only |
| Migration 0001 | `internal/adapters/postgres/migrations/0001_init.up.sql` | EXTEND (one-time edit) | NOLOGIN role; dynamic grant; `.down.sql` reviewed for consistency |
| `Secret` | `internal/support/` | EXTEND | Redacts under `String`, `GoString`, `slog.LogValuer` |
| Console assets seam | `internal/adapters/http/console_static.go`, `router.go`, plus a `console_embed`-tagged file pair and gitignored `console_dist/` in the same package | EXTEND | `fs.FS` into `mountConsole`; no new package |
| Release classifier | `internal/release/`, thin `scripts/releaseplan/main.go` | CREATE NEW | Pure |
| Smoke harness | `scripts/smoke/main.go` | CREATE NEW | Go, per the `scripts/race` convention; used by `release.yml` and local runs |
| Quickstart checker | `scripts/quickstart/main.go` | CREATE NEW | Extracts the tagged fenced command blocks of the quickstart and runs them in order; shares helpers with the smoke harness |
| Release workflow | `.github/workflows/release.yml` | CREATE NEW | Path is a public interface; the quickstart job runs last |
| CI workflow | `.github/workflows/ci.yml` | EXTEND | `workflow_call`; branches-only `push:` |
| Dockerfile | root | EXTEND | See DDD-34 |
| `.dockerignore` | root | CREATE NEW | None exists; controls the build context |
| Compose files | `docker-compose.yml`, `docker-compose.loadtest.yml` | EXTEND | `migrate` service runs `ledgerops migrate` from the built image |
| Quickstart for the published image | `docs/quickstart-published-image.md` | CREATE NEW | US-8; fenced blocks tagged for the checker; includes verify, upgrade, rotation, dirty-state recovery |
| Published-image run example | `examples/published-image/docker-compose.yml`, `examples/published-image/README.md` | CREATE NEW | Separate from the demo compose; no demo credentials; secrets generated by documented commands |
| README | `README.md` | EXTEND | Quickstart leads with the published image; limitation line removed; "Adoptable" roadmap item amended |

The domain core, application layer and ports are untouched.

## Wave: DESIGN / [REF] Driving ports

| Port | Surface | Contract |
|---|---|---|
| `ledgerops migrate` | container command | Env: `LEDGEROPS_MIGRATE_DSN`, `LEDGEROPS_APP_PASSWORD` xor `_FILE`. Exit 0 with schema version and "app role password set/updated"; non-zero with named causes. Never prints secrets |
| `ledgerops serve` / no argument | container command | Env: `LEDGEROPS_APP_DSN`, `LEDGEROPS_OPERATOR_KEY` required; `LEDGEROPS_DEMO_TENANT_KEY`, `LEDGEROPS_ADDR` optional; refuses if `LEDGEROPS_MIGRATE_DSN` is present |
| `ledgerops version` | container command | `ledgerops <version> (commit <short>)` or `dev (commit unknown)`; needs no env |
| `git push` of `vMAJOR.MINOR.PATCH[-pre]` | Actions trigger | Only entry to `release.yml` |
| `docker pull ghcr.io/gizawnahom/ledgerops:<tag>` | registry | Tags per `ReleasePlan` |
| `cosign verify`, `cosign verify-attestation` | registry artifacts | Identity regex in ADR-021 |
| `GET /health`, `GET /console`, `GET /console/verdict` | HTTP (existing) | Unchanged |

The serve path exposes no write method to migration state, and the migrate path is a separate driving port; read and write are not combined on one interface.

## Wave: DESIGN / [REF] Driven ports + adapters

| Port (outbound) | Adapter | Probe (Earned Trust) |
|---|---|---|
| `SchemaMigrator` (privileged DSN) | `postgres` migrator | `FactReader` (read-only catalog queries) and `PreflightProber` (write-capable empirical probes, rolled back, never committed) (ADR-022). Fault injections: no `CREATEROLE`, no `ADMIN OPTION` on existing role, not database owner, no `CREATE` on `public`, table owned by another role, second database in a cluster with the role. Dirty-state detection before any change |
| `RolePasswordSetter` (privileged DSN) | `postgres` role adapter | Accepts a finished SCRAM verifier only; probe: a real client login with the plaintext succeeds after the set (post-condition check in the acceptance suite and the smoke) |
| App store (app DSN) | existing `postgres.Open` | Existing `probeStartup` (SELECT, UPDATE-on-entries refused) + SQLSTATE 28P01 classification |
| Console assets (`fs.FS`) | embedded FS, or `os.DirFS` | In a `console_embed` build, `index.html` must exist or refuse to start |
| Password file read | OS file read in the shell | Strips one trailing newline; unreadable file is a named config error (documents the 0400-root trap) |
| Registry, signing, release APIs | GitHub Actions steps | Verification steps: `cosign verify` (index and platforms), `verify-attestation` (platform digests), `imagetools inspect` (platforms, labels), anonymous pull of the digest before any tag is promoted, release body contains the image reference |

Probe contract enforcement: subtype check (Go interface assertion `var _ Prober = (*migrator)(nil)` at the composition root), structural check (AST test that every driven adapter type exposes a probe), behavioural check (fault-injection acceptance tests above). Self-application: the structural test fails if a new adapter lacks a probe.

## Wave: DESIGN / [REF] Technology choices

| Item | Choice | License |
|---|---|---|
| Language/runtime | Go 1.25 (existing), `CGO_ENABLED=0` | BSD-3 |
| Migration engine | golang-migrate v4 (existing, embedded via iofs) | MIT |
| Base image | alpine:3.20 (existing); every `FROM` (and smoke `postgres:16`) pinned by digest, values by DEVOPS | MIT |
| Actions | every `uses:` pinned by full commit SHA; cosign 2.x, syft, buildx versions pinned | Apache-2.0 / MIT |
| Console build | Node 20 stage, Vite 5 (existing) | MIT |
| Registry | GHCR via `GITHUB_TOKEN` (D1) | n/a (service) |
| Image build | docker/buildx, docker/metadata-action | Apache-2.0 |
| Signing | cosign keyless, Sigstore Fulcio/Rekor | Apache-2.0 |
| SBOM | syft, SPDX-JSON | Apache-2.0 |
| Password verifier | SCRAM-SHA-256: PBKDF2-HMAC-SHA-256 and HMAC from Go stdlib; SASLprep via `golang.org/x/text/secure/precis` (module already in `go.mod` v0.40.0 as `// indirect` via pgx; becomes a direct dependency, so `go.mod` is edited) | BSD-3 |

## Wave: DESIGN / [REF] Decisions table

| DDD | Locked? | Origin | ADR |
|---|---|---|---|
| DDD-27..29, 41, 47, 48 | Yes | OQ-2, user-accepted; DDD-41 is a user decision (review cycle 1) | 017 |
| DDD-30..32 | Yes | OQ-3 | 018 |
| DDD-33..34 | Yes | OQ-5, non-root | 019 |
| DDD-35..37, 44..46 | Yes | OQ-6, review cycle 1 | 020 (044 also 019) |
| DDD-38, 43 | Yes | OQ-10, review cycle 1 | 021 |
| DDD-39, 42 | Yes | OQ-8, review cycle 1 | 022 |
| DDD-40 | Yes | OQ-9 | 023 |

## Wave: DESIGN / [REF] Reuse Analysis

| Existing component | File | Overlap | Decision | Justification |
|---|---|---|---|---|
| Embedded migration runner | `internal/adapters/postgres/store.go` | Embedded SQL up as the privileged role | EXTEND | Same engine; adds password parameter, provisioning, probe, dirty detection |
| Migration 0001 | `.../migrations/0001_init.up.sql` | Password literal, hardcoded grant | EXTEND | A new migration cannot fix it (0001 fails first); one-time edit, fixture + checksum guard |
| Composition root | `cmd/api/main.go` | Env reads, startup probe | EXTEND | Dispatcher, pure config, classifier |
| `mountConsole` / `consoleDistDir` | `internal/adapters/http/console_static.go` | SPA serving | EXTEND | `fs.FS` seam; dev behaviour retained |
| Embed file pair (`console_embed` / `!console_embed`) | `internal/adapters/http/` (new files, same package) | Where the bundle enters the binary | EXTEND (new files in an existing package) | A separate `consoledist` package was considered and rejected: the HTTP adapter already owns console serving, `go:embed` needs the bundle inside the embedding package's directory, and a tagged file pair adds no import edge |
| `.dockerignore` | root | Build context hygiene | CREATE NEW | No file exists; without it `.git`, `node_modules` and docs enter the context and defeat stage caching |
| README quickstart and limitations | `README.md` | Documents the build-from-source path | EXTEND | Leads with the published image; limitation removed |
| Quickstart for the published image; example compose | `docs/quickstart-published-image.md`, `examples/published-image/` | Overlaps README quickstart and the demo `docker-compose.yml` | CREATE NEW | The demo compose bakes demo credentials and builds from source (D6 keeps it unchanged); the published-image path must use generated secrets and the pulled image, so it cannot extend the demo files without breaking `make up` |
| `support` package | `internal/support/` | Hash primitive, secret hygiene | EXTEND | Add `Secret` |
| `Dockerfile` | root | Image build | EXTEND | Stages added |
| Compose files | root | Demo migrate step | EXTEND | `migrate/migrate` replaced by the image's own `migrate` so demo CI exercises shipped code |
| `ci.yml` | `.github/workflows/` | Gate reused for release | EXTEND | `workflow_call`, branches-only `push:` |
| `release.yml` | new | Tag pipeline | CREATE NEW | No existing release workflow; `nightly.yml`/`load-test.yml` differ in trigger and purpose |
| `internal/release` | new | Tag classification, highest stable | CREATE NEW | No alternative; `metadata-action` cannot express the backport rule; testable pure function |
| Smoke harness (`scripts/smoke`) | new | Pull-migrate-run-assert | CREATE NEW | `scripts/race` is the only existing script and is a race runner against the compose stack; `Makefile` `demo-*` drive compose from source and cannot target a registry digest. Follows the Go-program-per-directory convention and reuses the repository's testcontainers dependency |
| Quickstart checker (`scripts/quickstart`), release planner (`scripts/releaseplan`) | new | Run documented commands verbatim; execute the pure plan | CREATE NEW | No existing equivalent; both are thin mains over shared helpers or `internal/release` |

### Contract shapes (per overlapping component)

Only three shapes are used: pure-function (return-only), bounded-change (declared mutation set over a stated universe), unbounded-preservation (must return a Plan, never mutate). Every component of this feature has a row.

| Component | Shape | Universe and assertion mechanism |
|---|---|---|
| Argument dispatcher | pure-function | argv in, command value (or usage error) out. Table tests |
| `LoadServeConfig`, `ParseMigrateConfig` | pure-function | Env snapshot in, config plus all issues out. Table tests; property: blank, whitespace-only and unset are equivalent; both-password-sources refuses |
| Password-file reader | pure-function over an injected `ReadFile` capability | Returns a `Secret` or a named error; strips exactly one trailing newline. Table tests with a fake reader (LF, CRLF, none, empty, unreadable) |
| `Secret`, DSN redactor | pure-function | No formatting path prints the value (`%v`, `%+v`, `%#v`, slog JSON); redactor property: output never contains the password component |
| Startup-refusal classifier | pure-function | (error, DSN user) in, `Refusal{reason, role}` out; sentinel-password assertion on every output |
| Dirty-state detector | pure-function | (version, dirty) in, refusal naming the version out |
| SCRAM-SHA-256 verifier builder | pure-function with injected salt and iteration count | Deterministic vectors; SASLprep cases; a real-client login after the set (acceptance) |
| `PlanMigration` | pure-function (unbounded-preservation producer) | `Facts` plus config in, `Refuse(reason)` or ordered `Plan` out; never mutates. Table tests cover every refusal reason in ADR-022 and the cluster rule (DDD-41) |
| Plan executor (`Migrate`/`MigrateStep` shell) | bounded-change | Declared delta: the role claim (when absent), objects created by pending migration files, `ledgerops_app` login attribute and rolpassword, and the `GRANT CONNECT` entry in the target database's ACL. Complement universe (asserted by a before/after snapshot diff in the upgrade and fresh-database acceptance runs): `pg_authid` (every role other than `ledgerops_app` identical, including `rolpassword`, `rolcanlogin`, `rolsuper`), `pg_shdescription` (every other shared object's comment), `pg_auth_members`, `pg_default_acl`, `pg_namespace` (+ `nspacl`), `pg_class` (relname, relkind, relowner, relacl: only declared additions), `pg_database` (every other database and its ACL identical), plus data: rows of `entries`, `accounts`, `transactions`, `idempotency_keys`, `tenants`, `tenant_links`, `counterparty_aliases`, `transfer_state` unchanged (counts and checksums) and trial balance zero |
| `FactReader` | bounded-change, empty delta (read-only) | Pre/post snapshot over the universe above is identical; the port exposes no statement other than SELECT (structural test) |
| `PreflightProber` | bounded-change, zero net change; explicitly write-capable, separately named port | Its statements are DDL run in a transaction that is rolled back, never committed; pre/post catalog snapshot identical (OID and txid consumption excluded as unobservable) |
| Role claim | bounded-change | Declared delta: creation of `ledgerops_app` NOLOGIN and its owner-database marker (`pg_authid`, `pg_shdescription`) in its own committed transaction, only when the role is absent; atomic by the unique index on `pg_authid` (DDD-48). Complement: nothing else changes; concurrency scenario asserts exactly one winner |
| `RolePasswordSetter` | bounded-change | Declared delta: `ledgerops_app` `rolcanlogin` and `rolpassword` only; same complement universe |
| `SchemaMigrator` | bounded-change | Declared delta: pending migration files only; same complement universe; dirty-state premise test (kill mid-file) |
| `serve` | bounded-change | Declared delta: rows written through the existing ledger ports as `ledgerops_app`; it can run no DDL and no role statement (grants), and holds no migrate DSN (refuses if present). Complement-universe assertion: an acceptance run drives postings through a running `serve`, then diffs the catalog snapshot (above) and the data tables, expecting only ledger-port row additions; a second check attempts `CREATE TABLE`, `ALTER ROLE` and `UPDATE`/`DELETE` on `entries` as the app role and expects refusal (existing `probeStartup` covers the last at startup) |
| `refuseIfMigrateDSNPresent` | pure-function | Env snapshot in, optional refusal out; presence only, never reads the value |
| Console assets seam | bounded-change, empty declared delta over an injected read capability | Reads are effects, so the `fs.FS` is injected (embedded FS or `os.DirFS`); the interface has no write methods (compile-time); startup check that `index.html` exists, asserted by the embed and no-index tests |
| `internal/release` classifier | pure-function | (tag, existing stable versions) to `ReleasePlan`. Table and property tests on semver, prerelease, backport, re-run |
| Smoke harness, quickstart checker | bounded-change | Declared delta: ephemeral containers and databases it creates and removes; registry access is pull-only (job has `packages: read`); nothing outside its temp universe changes |

## Wave: DESIGN / [REF] Architecture Enforcement

Style: modular monolith, ports-and-adapters, functional core / effect shell. Language: Go. Tools: golangci-lint (`depguard`, alongside the existing `exhaustive`) and an AST/grep test.
- `internal/domain` and `internal/app` import no adapter package.
- `LEDGEROPS_MIGRATE_DSN` and `LEDGEROPS_APP_PASSWORD[_FILE]` are read only in the migrate command file. The single allowed exception is serve's `refuseIfMigrateDSNPresent` site, which may name `LEDGEROPS_MIGRATE_DSN` only through `os.LookupEnv` and never reads or forwards the value. The AST test names that site explicitly; the serve path does not import the migrator.
- `RolePasswordSetter` and `SchemaMigrator` are separate interfaces (neither embeds the other); the setter's signature accepts a verifier type, never a plaintext password.
- Every `uses:` in `release.yml` is pinned to a full commit SHA (zizmor or a grep test), and every `FROM` carries a digest.
- The Dockerfile's `go build` carries `-tags console_embed` (structural test).
- The DSN passed to golang-migrate never carries `x-multi-statement` (test), because the dirty-state analysis depends on one implicit transaction per file.
- `Secret` never renders via `%v`, `%+v`, `%#v` or slog JSON.
- Checksum guard on migrations 0001 (edited) through 0006.
- Every driven adapter exposes a probe (structural test), and fault-injection tests exercise it (behavioural).

## Wave: DESIGN / [REF] External integrations (handoff annotation for DEVOPS)

External Integrations Requiring Contract Tests:
- GHCR (OCI distribution API): publish, promote tags, anonymous pull. Pact does not apply to a registry; the consumer-driven checks are the release workflow's verification steps (`imagetools inspect` for platforms and labels, anonymous pull of the digest) and the quickstart job.
- Sigstore (Fulcio, Rekor, GitHub OIDC): keyless signing. Recommended: `cosign verify` (index and platform digests) and `verify-attestation` (platform digests) in the pipeline against the published identity regex.
- GitHub Releases/Actions APIs: release creation, prerelease flag, release body naming the image reference. Verified by the pipeline's release job.
- Docker Hub base images and PostgreSQL 16: pinned versions; smoke covers a real Postgres.

## Wave: DESIGN / [REF] Open questions

For DEVOPS:
1. arm64 smoke runner: `ubuntu-24.04-arm` (preferred for a public repo) versus QEMU fallback.
2. OQ-7 owner action: first GHCR push is private; the owner sets the package public and links the repository once. The anonymous-pull probe (now before any tag is promoted) fails the first run by design; the owner flips visibility and re-runs (the tag does not exist yet, so the re-run builds anew).
3. KPI-5 `demo_first_green_seconds` rises because `make up` / the `demo` job now build the console; re-baseline in `kpi-contracts.yaml`. Use BuildKit npm cache mounts.
4. Fill in digests for every `FROM` and the smoke `postgres:16`, SHAs for every `uses:`, pinned cosign/syft/buildx versions; dependabot/renovate for all of them (DDD-44); the unpinned-`uses:` check.
5a. Untagged-digest cleanup: none in v0.1.0 (DDD-45); revisit with a multi-arch-aware tool.
5b. Where the quickstart job runs beyond `release.yml` (for example `nightly.yml` against `latest`); the checker program is the same.
5. K1-K7 wiring into `docs/product/kpi-contracts.yaml`.
6. Slice 03 is amd64-only by the DISCUSS slicing; the design builds both architectures in one job. DEVOPS may stage arm64 in slice 06 or enable it from the start.

For DISTILL:
1. Frozen pre-feature fixture (0001-0006 old text) as the basis for the US-4 and smoke upgrade scenarios.
2. Two crash scenarios: kill between files (re-run completes) and kill mid-file (dirty refusal naming the version, role still NOLOGIN).
3. Non-superuser privilege fixtures: no `CREATEROLE`; no `ADMIN OPTION` on an existing role; not database owner.
4. Sentinel-password and sentinel-key assertions on every failure path (migrate, serve, probe, wrong password at start).
5. `serve` refuses when `LEDGEROPS_MIGRATE_DSN` is set (new scenario; see upstream-changes).
6. Secret file mounted `0400 root` under the non-root user: named error, documented trap.
7. Console embed (corrected in review cycle 1): with `console_dist` absent or empty, a `console_embed` build fails at `go build` (a Dockerfile-level or build test, not a startup test); with the directory present but no `index.html` the binary refuses to start; an untagged build serves from `consoleDistDir` as before; a structural test asserts the Dockerfile builds with the tag.
8. Cluster rule (DDD-41): a second, empty database in a cluster that already has `ledgerops_app` -> `migrate` refuses naming the conflict, nothing changed (role, password, schema, the first database untouched); a database that already has the ledgerops schema is not refused (rotation, upgrade, re-run).
9. Dirty-state premise: kill mid-file with `pg_terminate_backend` against a fixture migration; assert no partial effect and `dirty = true`; `migrate` then refuses naming the version.
10. SCRAM: non-ASCII password acceptance (composed and decomposed forms, fullwidth characters) with a real pgx login; deterministic vectors with injected salt.
11. Fault-injection privilege fixtures extended: no `CREATE` on `public`; a ledgerops table owned by another role; no `GRANT CONNECT` right on the database.
13. Concurrency (DDD-48): two fresh `migrate` runs started together against two different empty databases in one cluster -> exactly one succeeds, the other refuses with the cluster-rule message, neither database is left dirty, the role carries the winner's password only; two concurrent runs against the same database both end consistent (the loser resumes via the marker). A role whose marked database no longer exists: refusal names `DROP ROLE ledgerops_app` as the privileged remedy and changes nothing.
12. Release body names the image reference; a resumed run (tag exists) reuses the digest; anonymous-pull probe fails on a private package before any tag exists.

## Wave: DESIGN / [REF] Test harness impact (cluster rule, DDD-41)

Verified against `tests/acceptance/*`, `testhelper_test.go` and `trial_balance_tenant_scope_test.go` (review cycle 1):
- **Acceptance worlds** (`ledgercore`, `multitenancy`, `intertenanttransfer`): each boots one shared Postgres per test binary, calls `Migrate` once on the template database (pristine cluster, role absent: allowed), then gives every scenario a database made with `CREATE DATABASE ... TEMPLATE` and an explicit `GRANT CONNECT`. Cloning is SQL, not `migrate`, so the cluster rule never fires. These suites keep working; they only pass `app-secret` to the new signature.
- **Per-test containers** (`testhelper_test.go`, `trial_balance_tenant_scope_test.go`, `ledgercore` `StartWithoutSchema`/`MigrateFromZero`): one cluster per test; the role is absent; allowed.
- **New scenarios that need their own cluster**: anything that creates the role, sets or rotates its password, or exercises Rule A/B (US-1, US-4, dirty state) mutates a cluster-global object, so it runs in a dedicated container (tag `@dedicated-cluster`), as `StartWithoutSchema` already does. The cluster-rule refusal scenario may run in any cluster that already has the role by creating an empty second database and dropping it afterwards.
- **Deferred `LEDGEROPS_TEST_PG_DSN` shared-instance reuse** (no code exists yet; plan restated per user decision, review cycle 2): on the shared instance, one ledgerops database is migrated once and is the template. Subsequent runs skip `migrate` (the schema is present) and clone per scenario with `CREATE DATABASE ... TEMPLATE`. `@dedicated-cluster` scenarios get their own container, never the shared instance. That first ledgerops database is the only one allowed in that cluster (DDD-41/48: any other fresh `migrate` there is refused).
- **No test-only path** is added to the production binary: the refusal has no override, so production behaviour is not weakened to suit tests. Harnesses that need extra migrated databases use `CREATE DATABASE ... TEMPLATE`.

## Wave: DESIGN / [REF] Wave Decisions Summary

- Pattern: modular monolith, ports-and-adapters, functional core / effect shell (unchanged). Paradigm: functional.
- Key components: `ledgerops` binary (serve/migrate/version), postgres migrator + provisioner, embedded console, `internal/release`, `release.yml`, smoke harness.
- Constraints established: app never reads or holds the migrate DSN (and refuses if present); app role cannot log in on a database `migrate` created until the operator's password is set; **one ledgerops database per Postgres cluster** (fresh migrate refuses when `ledgerops_app` exists); applied migrations immutable after the 0001 exception; published version tags immutable by workflow policy (pin by digest); `release.yml` path frozen; all actions and base images pinned; no secret in any output.
- Upstream changes: see `design/upstream-changes.md` (items 1-8 applied by the PO; 9-12 new in review cycle 1).

## Wave: DESIGN / [WHY] Trade-off analysis

Rendered on request (expansion `trade-off-analysis`, user-approved). Quality attributes in rank order: security and supply-chain integrity (S), release-path reliability (R), operability (O), testability (T), portability (P), time-to-market (M). "+" helps, "-" costs, "0" neutral.

| Decision | Chosen | S | R | O | T | P | M | Main alternative and why not |
|---|---|---|---|---|---|---|---|---|
| Edit 0001 in place, NOLOGIN (DDD-27) | edit once | + (Rule A by construction) | 0 | + (any db name) | - (fixture, checksum guard) | + | 0 | New migration: cannot work, 0001 fails first |
| One ledgerops DB per cluster (DDD-41) | refuse | + (no inherited or shared credential) | + | - (second DB needs second cluster) | - (dedicated clusters for role-mutating tests) | - (managed instances hosting several apps' databases are fine, several ledgerops are not) | + (no role-per-db design) | Role per database: changes every grant and the public contract |
| SCRAM verifier transport (DDD-28, 47) | client-side verifier | + (no plaintext on wire or log) | 0 | 0 | - (SASLprep, salt injection) | 0 | - (about 25 lines plus tests) | Plaintext literal: leaks under `log_statement=ddl` |
| Dirty state: refuse, no force (DDD-29) | refuse | + (never guess on a ledger) | - (manual recovery on a rare event) | - (operator procedure) | + | 0 | + | Auto-force: state not derivable |
| Embed console behind a build tag (DDD-33) | embed | + (no silent 404) | + | + | 0 | + (single binary) | - (build-tag split) | Path convention: silent skip survives |
| Non-root numeric user (DDD-34) | non-root | + | 0 | - (0400-root secret trap) | 0 | 0 | 0 | Root: simpler, weaker |
| `serve` refuses when migrate DSN present (DDD-31) | refuse | + (OPS-10 enforced at runtime) | 0 | - (shared env files fail) | + | 0 | 0 | Trust the operator: the invariant is decoration |
| Digest-first release order (DDD-35) | digest-first | + (no unsigned public tag) | + (no partial public release) | - (first run fails by design; orphan digests) | 0 | 0 | - (more jobs) | Literal D8 order |
| Re-run: resume, no reproducible builds, no pruning (DDD-45) | resume | 0 | + (idempotent) | - (orphans accumulate) | 0 | 0 | + | Reproducible builds: large effort; prune: breaks indexes |
| SBOM per platform, verify by platform digest (DDD-43) | per platform | + (honest content) | 0 | - (two-step lookup for consumers) | 0 | + | 0 | Index-level SBOM: wrong for one platform |
| Pinning everything (DDD-44) | pin | + | + (reproducible pipeline) | - (bump churn) | 0 | 0 | - | Floating tags: supply-chain exposure in the signing pipeline |
| Privileged role: explicit minimum + facts/plan + empirical probes (DDD-39, 42) | tested minimum | + (least privilege) | + (fail before change) | + (named privilege) | + (pure plan) | + (managed Postgres) | - (more fixtures) | Superuser only: excludes managed Postgres |

Sensitivity points: DDD-41 (single cluster) and DDD-45 (resume vs rebuild) each affect more than one attribute and are the two decisions most likely to be revisited if adoption shows operators hosting several ledgers per cluster or re-running releases often. Risks: the first-release run fails by design (documented); GHCR tag immutability is policy not enforcement (mitigated by digest pinning and signatures); the dirty-state recovery is manual (rare by construction).
