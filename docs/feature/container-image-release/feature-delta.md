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
- D14 (amended 2026-10-17, user decision from DEVOPS) First image release is `v0.2.0` (release candidates `v0.2.0-rc.N`); release notes are GitHub auto-generated. The existing source-only pre-release `v0.1.0` stays untouched; the release classifier reads published versions from GHCR, so `v0.1.0` is invisible to it. Original text: "First version is `v0.1.0`". OQ-12 closed.
- D11 Ordering: walking skeleton is slices 01a, 02 and 03 (migrate, safe start, gated publish with smoke), so a usable rc image exists after three slices. Slice 01 was later split into 01a and 01b (D15). Hardening (upgrade, console, tags/arch, signing, docs) follows.
- D12 Two further jobs added beyond J11: J12 (provenance) and J13 (upgrade in place). Each is a distinct P2 outcome that the user's D7 and D8 choices imply.
- D13 One new journey, `docs/product/journeys/pull-and-run-ledgerops.yaml`, extends the SSOT; no existing journey changes.

## Wave: DISCUSS / [REF] Scope Assessment (Elephant Carpaccio Gate)

Result: oversized by heuristic, user-approved (2026-10-17) to keep as one feature with 8 slices of at most one day each; slice 01 was later split into 01a/01b (D15), giving nine slices of at most one day.

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
| Tag-triggered GitHub Release (06) | Pull exact tag, `docker pull` (03) | `migrate` with my password (01a) | Fail-fast config check (02) | Console at `/console` (05) | Migrate existing demo DB in place (04) |
| Minor and `latest` tags (06) | Multi-arch manifest (06) | One ledgerops database per cluster, any name for the first (01b) | `version` output (05) | Verify signature and SBOM (07) | Rotate password (04) |
| Prerelease handling (06) | Anonymous pull (OQ-7) | | | Quickstart for published image (08) | |

Walking skeleton (thinnest end-to-end): US-1a (migrate with a password), US-2 (refuse bad config), US-3 (gated publish and smoke, amd64, exact tag only). After these, `docker pull` then `migrate` then run then `/health` works from a registry.

Releases by outcome:
- Release A, "A client can run a pinned image": US-1a, US-1b, US-2, US-3. Outcome KPIs K1, K3. US-1b is not on the walking skeleton; it follows US-1a and is delivered before US-4.
- Release B, "Existing and operator users are safe": US-4, US-5. KPIs K4, K6.
- Release C, "Looks like a real release": US-6, US-7, US-8. KPIs K2, K5, K7.

## Priority Rationale

1. Slice 01a first: the password literal in `0001` is the highest-uncertainty change (it touches an applied migration) and everything else depends on `migrate` existing. If setting a password Go-side breaks the append-only guarantee or the demo path, we learn it on day one.
1b. Slice 01b right after 01a, before slice 04: the cluster rule is a refusal layered on `migrate`, off the walking-skeleton path, and slice 04's upgrade must know whether it may touch an existing role. It carries the DDD-48 atomic-claim risk, so it is learned early but does not block slices 02 and 03, which can run in parallel with it.
2. Slice 02 second: tiny, no dependency, and it removes the failure that would otherwise make the smoke test in slice 03 meaningless (an image that starts with no operator key).
3. Slice 03 third: proves the pipeline end to end on an exact tag only. Biggest unknown in CI (workflow_call reuse of a compose-based `ci.yml`), so it comes before polish.
4. Slice 04 next: the upgrade promise (D8) protects people who already run the demo stack; it reuses 01a's code, 01b's role-ownership rule, and the 03 pipeline.
5. Slice 05: console and version. Independent of the others; placed after the pipeline exists so the smoke test can assert `/console`.
6. Slice 06: full tag set, arm64, labels, GitHub Release. Pure packaging polish over a working pipeline; moves `latest` for the first time.
7. Slice 07: signing and SBOM. Highest external-tooling risk (keyless identity, permissions) with the lowest effect on whether a client can run the image, so it comes after `latest` exists but before docs freeze the verify command.
8. Slice 08 last: documentation must name real tags and the real signing identity.

## Wave: DISCUSS / [REF] User stories with elevator pitches

Every story has a real user-invocable entry point and an observable result. No story is `@infrastructure`. Image and command names are illustrative; DESIGN settles exact names (OQ-3). Passwords below are fixtures, not secrets.

### US-1a — Migrate a fresh database with my own app password

job_id: J11 | persona: P1 | slice: 01a (split from the former US-1; the cluster rule is US-1b)

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
- [ ] The password never appears in command output or logs, with or without failure
- [ ] `docker-compose.yml`, the loadtest compose, the test helper and CI set `LEDGEROPS_APP_PASSWORD=app-secret` explicitly and `make demo-01..04`, `chaos-01`, `race-02`, `race-03` pass unmodified
- [ ] An app started with the wrong password exits non-zero with a clear refusal and no password in the log

Outcome KPIs
- Who: integrating developers preparing a database from the image
- Does what: reach a migrated database with their own password without editing SQL
- By how much: 100% of fresh-database acceptance runs end with the supplied password working and `app-secret` refused (K3)
- Measured by: acceptance suite against a Testcontainers Postgres, and the release smoke test
- Baseline: 0% (the password is fixed in SQL)

Technical Notes
- Changes an already-applied migration's content (`0001`): safe handling is OQ-2. The compose `migrate` service moves from `migrate/migrate` to this subcommand, because `migrate/migrate` cannot set the password; demo CI then exercises the same code the release ships.
- The privileged role needs enough privilege to create and alter roles (OQ-8). `migrate` runs a preflight privilege probe first: an insufficient privilege fails before any schema change with "no changes applied".
- A secret file owned by root with mode 0400 is unreadable by the non-root image user; the named error documents this trap.
- The cluster rule (one ledgerops database per cluster) is US-1b, which builds on this story.
- Dependencies: none inside this feature; depends on shipped `ledger-core`.

### US-1b — One ledgerops database per cluster

job_id: J11 | persona: P1 | slice: 01b | depends on: US-1a

Problem: Tomasz Kowalski shares one Postgres cluster at Kestrel between teams. The role `ledgerops_app` is cluster-global, so running `migrate` for a second ledgerops database in the same cluster would silently share (and on rotation, change) one login across both databases. Today nothing tells him, and he finds out when one team's password rotation locks the other team's service out.

#### Elevator Pitch
Before: Tomasz cannot tell that a second ledgerops database in the same cluster would share the app role and its password with the first.
After: run `docker run --rm -e LEDGEROPS_MIGRATE_DSN=postgres://kestrel_admin:...@db/kestrel_ledger_2 -e LEDGEROPS_APP_PASSWORD_FILE=/run/secrets/ledgerops_app_password -v ./secrets:/run/secrets ledgerops:dev migrate` against a cluster whose `kestrel_ledger` is already migrated → sees a refusal naming the conflict and the remedies, exit non-zero (wording illustrative)
Decision enabled: Tomasz decides whether to provision a separate cluster, migrate the database that owns the role instead, or drop an orphaned role.

Domain Examples
1. Priya's cluster has no ledgerops role; she migrates `kestrel_ledger` (not the name `ledgerops`) and it works: the first database in a cluster can have any name.
2. Tomasz's cluster already has `kestrel_ledger` migrated; he tries `kestrel_ledger_2`. Migrate refuses, nothing changes, and re-running migrate against `kestrel_ledger` is still allowed.
3. Two engineers start `migrate` for `kestrel_ledger` and `kestrel_ledger_2` at the same moment on a fresh cluster. Exactly one succeeds; the other refuses and leaves no dirty state. A third case: `kestrel_ledger` was dropped but the role remains; the refusal names `DROP ROLE ledgerops_app` as the privileged remedy.

UAT Scenarios (BDD)

Scenario: The first ledgerops database in a cluster can have any name
Given the cluster has no `ledgerops_app` role and the database is named `kestrel_ledger`, not `ledgerops`
When Priya runs `migrate`
Then it succeeds and `ledgerops_app` can connect to `kestrel_ledger`

Scenario: A second ledgerops database in the same cluster is refused
Given the cluster's database `kestrel_ledger` is already migrated, so the role `ledgerops_app` exists
When Tomasz runs `migrate` against a second, empty database `kestrel_ledger_2` in that cluster
Then it exits non-zero naming the conflict
And the role, its password, `kestrel_ledger` and `kestrel_ledger_2` are all unchanged
And running `migrate` again against `kestrel_ledger` itself (a database already carrying the schema) is not refused
And the message names the remedy: migrate the database that owns the role, use a separate cluster, or, when that database no longer exists, run `DROP ROLE ledgerops_app` as a privileged role

Scenario: Two simultaneous first migrations in one cluster cannot both claim the role
Given two empty databases `kestrel_ledger` and `kestrel_ledger_2` in one cluster
When two `migrate` runs start at the same time
Then exactly one succeeds and the other refuses with the cluster-rule message
And neither database is left in a dirty migration state

Acceptance Criteria
- [ ] Migration works for a database with any name, for the first ledgerops database in a cluster
- [ ] A fresh `migrate` against an empty database in a cluster where `ledgerops_app` already exists exits non-zero naming the conflict, with the role, its password and both databases unchanged; a database already carrying the ledgerops schema (upgrade, rotation, re-run) is not refused; the message names the remedies (migrate the owning database, use a separate cluster, or `DROP ROLE ledgerops_app` as a privileged role when the owning database no longer exists)
- [ ] Two simultaneous `migrate` runs against two empty databases in one cluster: exactly one succeeds, the other refuses with the cluster-rule message, and neither database is left dirty

Outcome KPIs
- Who: operators sharing a Postgres cluster
- Does what: learn at migrate time that a second ledgerops database would share the app role, instead of through a lockout
- By how much: 100% of second-database acceptance runs refuse with the role, password and both databases unchanged; 0 concurrent runs both succeed
- Measured by: acceptance suite in a dedicated Testcontainers cluster
- Baseline: 0% (no detection today)

Technical Notes
- User decision (DESIGN review cycle 1): one ledgerops database per Postgres cluster, because the role `ledgerops_app` is cluster-global. The role claim is atomic (DDD-48), so concurrent first migrations cannot both succeed.
- Needs `migrate` from US-1a. Scenarios that create the role mutate a cluster-global object and need their own cluster.
- Dependencies: US-1a.

### US-2 — Know immediately when the image is misconfigured

job_id: J11 | persona: P1 | slice: 02

Problem: Priya starts the image with only a database DSN. Today the server starts normally and answers 401 on every operator route, because an unset `LEDGEROPS_OPERATOR_KEY` makes no token valid (`router.go:291-293`); she finds out only when `POST /tenants` fails, and cannot tell whether the key, the route or the build is at fault. An unset `LEDGEROPS_APP_DSN` is not validated either.

#### Elevator Pitch
Before: a misconfigured image starts and then fails confusingly at the first operator call, or points at the wrong database.
After: run `docker run --rm ghcr.io/gizawnahom/ledgerops:0.2.0` with no settings → sees `config invalid: LEDGEROPS_APP_DSN is required; LEDGEROPS_OPERATOR_KEY is required` and exit 1 (wording illustrative)
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
After: run `docker pull ghcr.io/gizawnahom/ledgerops:0.2.0` → sees `Status: Downloaded newer image for ghcr.io/gizawnahom/ledgerops:0.2.0`
Decision enabled: Priya decides to adopt `0.2.0` as her pinned version and builds her deployment on it.

Domain Examples
1. Gizaw pushes `v0.2.0` on a green commit (the earlier source-only `v0.1.0` is untouched). After the pipeline finishes, `0.2.0` is pullable anonymously and the smoke check passed.
2. The integration job fails on the commit tagged `v0.2.1`. No image is published and the run is red on the failing job.
3. Gizaw pushes `nightly-2026-10-17` and the branch `fix-typo`. Nothing is published.

UAT Scenarios (BDD)

Scenario: A release tag publishes a pullable image
Given main is green at the commit tagged `v0.2.0`
When the release pipeline finishes
Then `ghcr.io/gizawnahom/ledgerops:0.2.0` can be pulled without logging in

Scenario: A red build never publishes
Given an integration test fails at the commit tagged `v0.2.1`
When the pipeline runs
Then no image exists for `0.2.1` and the run is red

Scenario: A pushed image is proven usable before it gets a version tag
Given the image for `0.2.0` has been pushed by digest and no version tag exists yet
When the pipeline's check pulls that digest for each published architecture, migrates a fresh Postgres and starts it
Then `/health` answers 200 and only then is `0.2.0` tagged

Scenario: A version that fails its usability check is not presented as a release
Given the pushed image cannot migrate a fresh database
When the check fails
Then the run ends red naming the failing step and no version tag exists for it

Scenario: Only strict version tags publish
Given Gizaw pushes the tag `v1.2`, the tag `nightly-2026-10-17`, and a branch
When the pipelines run
Then no image is published for any of them

Scenario: A published version is never silently overwritten
Given `0.2.0` is already published
When the tag `v0.2.0` is moved to another commit and pushed
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
- Dependencies: US-1a (migrate), US-2 (fail-fast), both in the same feature.

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
- Dependencies: US-1a and US-1b.

### US-5 — Open the console and see the version from the image

job_id: J11 | persona: P2 (also serves P1) | slice: 05

Problem: The README promises an operator console, but the image has no Node build stage, so a client running only the image gets a `/console` that 404s with no error (`console_static.go:45-49`). Nor can Tomasz tell from a running container which release it is.

#### Elevator Pitch
Before: the console is unreachable from a pulled image and there is no way to read the running version.
After: run `curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/console` against the running image → sees `200`; run `docker run --rm ghcr.io/gizawnahom/ledgerops:0.2.0 version` → sees `0.2.0 (commit 3f9c2ab)` (wording illustrative)
Decision enabled: Tomasz decides whether this container is the release he intends and opens the console to check the books.

Domain Examples
1. Tomasz runs the `0.2.0` image and loads `/console` in a browser; it shows the verdict page after he enters the operator key.
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
Given an image built for `0.2.0` at commit `3f9c2ab`
When Tomasz runs the `version` command
Then it prints `0.2.0` and `3f9c2ab`

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
3. `v0.3.0` is released and later `v0.2.2` is tagged as a fix for the 0.2 line: `0.2` moves to `0.2.2`, `latest` stays on `0.3.0`.

UAT Scenarios (BDD)

Scenario: A stable release gets exact, minor and latest tags
Given no image has been published yet (the source-only `v0.1.0` release does not count)
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
Given `latest` is `0.3.0` and `0.2` refers to `0.2.1`
When `v0.2.2` is released
Then `0.2` moves to `0.2.2` and `latest` is still `0.3.0`

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
After: run `cosign verify ghcr.io/gizawnahom/ledgerops:0.2.0 --certificate-identity-regexp '^https://github.com/gizawNahom/ledgerops/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+(-.+)?$' --certificate-oidc-issuer https://token.actions.githubusercontent.com` → sees `The cosign claims were validated` and the signing identity (wording illustrative)
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
After: follow `docs/quickstart-published-image.md` from `docker pull ghcr.io/gizawnahom/ledgerops:0.2.0` → sees a first transfer's JSON response with a transaction id
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
- [ ] Upgrade, rotation and verification sections exist; the verify command uses the identity regex from ADR-021 with the exact-case owner `gizawNahom` (the image reference stays lowercase, `ghcr.io/gizawnahom/ledgerops`, because GHCR requires it)
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

1. All UAT scenarios across US-1a, US-1b and US-2 to US-8 pass (green)
2. Supporting tests pass (unit for config validation; integration for `migrate` and password handling against Postgres; workflow checks)
3. Code refactored, reviewed and approved
4. Merged to main
5. Demoable: from a clean machine with Docker, pull the latest rc, migrate, start and post a transfer
6. A real rc tag (`v0.2.0-rc.1`) has been published, smoke-passed and, once slice 07 lands, verified. The first release run is expected to fail at the anonymous-pull check until the owner sets the GHCR package public (OQ-7); the owner then re-runs
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

A: thin, sequential slices with no environment switch. Walking skeleton is slices 01a, 02 and 03 (01b follows 01a). The password, keys and DSNs are ordinary configuration, not an environment-switching mechanism.

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

Open questions handed to DESIGN. Closed by the user on 2026-10-17: OQ-1 (demo key optional), OQ-11 (keep one feature), OQ-12 (auto-generated notes; first version later amended from v0.1.0 to v0.2.0, D14). OQ-7 remains an owner action.

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
11. OQ-12 CLOSED: GitHub auto-generated release notes; first version `v0.2.0` (amended from `v0.1.0`, D14).

Dependencies on prior features: `ledger-core`, `multitenancy`, `inter-tenant-transfer`, `ledger-core-console` are shipped; nothing blocks this feature.

## Wave: DISCUSS / [REF] Requirements Completeness

Functional: covered, US-1a, US-1b and US-2 to US-8, 3 to 9 scenarios each. Non-functional: security (password and key handling, no secrets in logs, provenance), reliability (idempotent migrate, smoke before `latest`, immutable versions), performance (smoke at most 120 s; image size recorded, not gated), compatibility (demo path unchanged), operability (clear config errors). Business rules: tag classification and `latest` semantics explicit in US-6. Estimated completeness score: 0.96. The remaining gap is the eleven open items above, each named and owned rather than assumed.

## Wave: DISCUSS / [REF] Definition of Ready (DoR) Validation

| DoR item | Status | Evidence |
|---|---|---|
| 1. Problem statement clear, domain language | PASS | Each story opens with a named person and a concrete pain with file refs where relevant |
| 2. Persona with specific characteristics | PASS for P2 (real-project-owner); PARTIAL for P1 (archetype-unvalidated, inherited from `integrating-developer.yaml`) | Priya Nair and Tomasz Kowalski used consistently |
| 3. 3+ domain examples with real data | PASS | Every story has 3, with real names, tags, passwords and counts (1,240 entries; v0.2.0) |
| 4. UAT in Given/When/Then, 3 to 7 scenarios | PASS except US-2 (documented exception) | US-1a: 7, US-1b: 3, US-2: 9 (exception), US-3: 6, US-4: 6, US-5: 5, US-6: 7, US-7: 5, US-8: 7 |
| 5. AC derived from UAT | PASS | Each AC list maps to its scenarios |
| 6. Right-sized, 1 to 3 days | PASS per story (each slice at most 1 day); feature-level scope is oversized by heuristic, user-approved to keep as one feature (OQ-11 closed) |
| 7. Technical notes: constraints and dependencies | PASS | Per-story notes, Pre-requisites OQ-1 to OQ-12 |
| 8. Dependencies resolved or tracked | PASS with one owner action | Shipped features resolved; GHCR package visibility (OQ-7) tracked as an owner action before US-3 can be fully verified |
| 9. Outcome KPIs with measurable targets | PASS | Per-story KPIs and K1 to K7 |

DoR status: PASSED for all nine stories (US-1a, US-1b, US-2 to US-8). Re-check 2026-10-19 after DEVOPS back-propagation (first release v0.2.0, exact-case identity regex; US-2, US-3, US-5, US-6, US-7, US-8 fixtures shifted): only fixture values changed, no scenario was added or removed, so counts and all DoR items are unchanged (US-2 remains the documented 9-scenario exception). Re-run 2026-10-17 after the US-1 split (user-approved): US-1a has 7 scenarios and US-1b has 3, both within 3-7, so the US-1 scenario-count exception is retired. Remaining exception: US-2 at 9 scenarios (variants of one startup-config rule, one day or less). All other DoR items pass for US-1a and US-1b: each has a named problem, full elevator pitch, three domain examples, ACs from scenarios, J11, KPI (US-1a to K3, US-1b a cluster-refusal KPI added to the K3 family), technical notes and a tracked dependency (US-1b on US-1a). History of earlier re-runs (the first mentions of US-1 counts refer to the story before the split): re-run after review-cycle-1 items 9-13 (US-1, 3, 6, 7, 8): items 1-3, 5, 7-9 still PASS; scenario counts now US-1 9 (exception, same reasoning as below), US-3 6, US-6 7, US-7 5, US-8 7 (within 3-7). Re-run 2026-10-17 after the DESIGN back-propagation edits (US-1, 2, 3, 4, 7, 8): items 1-3, 5, 7-9 still PASS. Item 4/6 scenario count is now over the 3-7 guideline for two stories: US-1 has 8 and US-2 has 9 (US-3 6, US-4 6, US-7 5, US-8 6 are within). The extra scenarios are variants of the migrate-safety rule (US-1) and the startup-config rule (US-2); slice effort is unchanged at one day or less. PO accepts this as a documented exception, to be revisited by DISTILL, which may split a story's scenarios across two acceptance features without changing slices. Earlier amendment note follows. Re-run 2026-10-17 after the amendments: still PASSED. US-2 now has 8 scenarios (within the 3 to 7 guideline by one; the three demo-key scenarios are short variants of one rule, so the story is not split, noted for DESIGN). Carried: P1 archetype caveat (inherited), OQ-7 owner action. OQ-1, OQ-11, OQ-12 closed. Peer review (`nw-product-owner-reviewer`) was not run in this dispatch; it is optional per the DISCUSS skill and the mandatory consolidated review fires at the end of DISTILL.

## Wave: DISCUSS / [REF] Wave Decisions Summary

# DISCUSS Decisions — container-image-release

## Key Decisions
- [D1-D9] User answers as listed in Locked decisions; D9 revised 2026-10-17 (demo tenant key optional)
- [D10] Scope oversized by heuristic, user-approved to keep as one feature with 8 slices of at most one day (OQ-11 closed)
- [D14] First image release v0.2.0 (amended 2026-10-17; source-only v0.1.0 untouched), GitHub auto-generated release notes (OQ-12 closed)
- [D11] Walking skeleton is slices 01a, 02 and 03, ordered by uncertainty
- [D15] Slice 01 / US-1 split into 01a/US-1a (migrate with own password) and 01b/US-1b (one ledgerops database per cluster) on 2026-10-17, user-approved, because US-1 reached 10 scenarios. The feature now has nine slices of at most one day each.
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

Handoff to `nw-solution-architect` (DESIGN, full artifact set) and `nw-platform-architect` (DEVOPS, KPI sections only). Deliverables: this file, `slices/slice-01a-*.md`, `slice-01b-*.md` and `slice-02..08-*.md`, and the SSOT updates above. Peer review not yet run. No user answers outstanding; OQ-7 is an owner action.

---

## Wave: DISCUSS / [HOW] Gherkin scenarios

Rendered on request (expansion `gherkin-scenarios`, triggered by AC ambiguity). Declined by the user: `alternatives-considered`, `migration-playbook`.

The ambiguity: US-1a and US-4 both say "the app role never accepts a password the operator did not supply". Readers could disagree about two moments. The rule below settles them, stated as observable behavior; how it is achieved (OQ-2) is DESIGN's.

Rule A (the window during migrate): from the moment a fresh `migrate` run starts until it finishes or dies, a login as `ledgerops_app` is accepted only with the operator's password or not at all. `app-secret` is never accepted on a database this run created, at any instant, including after a crash.
Rule B (the pre-rotation existing database): a database created before this feature already accepts `app-secret`. That is existing state, not a violation. It stays so until a `migrate` run with an explicit password completes the password step; if that run fails or is interrupted before the password step, `app-secret` still works and the output says so. Rule A applies only to databases the run creates.

```gherkin
Feature: Container image release

  # --- US-1a / Rule A: the window during migrate on a fresh database
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
    Given main is green at the commit tagged "v0.2.0"
    When the release pipeline finishes
    Then "ghcr.io/gizawnahom/ledgerops:0.2.0" pulls without login
    And the smoke check ran against the pushed digest before "0.2.0" was tagged, migrated a fresh database and saw /health 200

  Scenario: A red build publishes nothing
    Given an integration job fails at the commit tagged "v0.2.1"
    When the pipeline runs
    Then no image exists for "0.2.1"

  Scenario: Non-version tags and branches publish nothing
    Given the tags "v1.2" and "nightly-2026-10-17" and a branch push
    When the pipelines run
    Then no image is published

  Scenario: A moved version tag never overwrites a published version
    Given "0.2.0" is published
    When "v0.2.0" is force-moved and pushed
    Then the pipeline refuses and the digest of "0.2.0" is unchanged

  Scenario: A prerelease never moves latest or the minor tag
    Given "latest" is "0.2.0"
    When "v0.3.0-rc.1" is released
    Then only "0.3.0-rc.1" is added and "latest" is still "0.2.0"

  Scenario: A backport moves its minor tag but not latest
    Given "latest" is "0.3.0" and "0.2" refers to "0.2.1"
    When "v0.2.2" is released
    Then "0.2" refers to "0.2.2" and "latest" is still "0.3.0"

  # --- US-5 / US-7: console, version, provenance
  Scenario: The image serves the console and reports its version
    Given the image for "0.2.0" at commit "3f9c2ab" is running
    When Tomasz requests /console and runs the version command
    Then /console returns 200 and version prints "0.2.0" and "3f9c2ab"

  Scenario: A failed console build yields no image
    Given the console build fails
    When the image is built
    Then the build fails

  Scenario: A released image verifies and a look-alike does not
    Given release "0.2.0" and a look-alike under the same name
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
| DDD-34 | Image build shape (amended 2026-10-19 from DEVOPS: `node:22-slim` console stage, `alpine:3.24` runtime, topology asserted by a structural test): one Dockerfile, Node stage, `$BUILDPLATFORM` cross-compile, ldflags stamping, alpine with no `RUN` in the final stage, numeric non-root `USER 65532:65532`, OCI labels, `.dockerignore` (ADR-019) | What is tested is what ships; no QEMU for compile; non-root for a ledger holding funds |
| DDD-35 | (amended 2026-10-19 from DEVOPS: one multi-platform build pushes the untagged index by digest, no per-platform matrix or merge job; provenance and SBOM attestations off in BuildKit) `release.yml` on version-tag push; `ci.yml` gains `workflow_call`, `push:` becomes branches-only; digest-first order (build, smoke, sign/attest/verify, anonymous-pull probe, then tags promoted last) (ADR-020) | No duplicate CI run; no public version tag before smoke, signature and public visibility |
| DDD-36 | `internal/release` pure classifier produces a `ReleasePlan`; workflow only executes it (ADR-020) | Backport/prerelease/`latest` rules are testable business logic, not inline shell |
| DDD-37 | Tag immutability guard (absent or same digest), global non-cancelling concurrency group, least-privilege per-job permissions (sign job: `packages: write` + `id-token: write`; smoke: `packages: read`), frozen `release.yml` identity (ADR-020) | Re-runs are idempotent; "highest stable" is evaluated serially; the signing identity includes the workflow path |
| DDD-38 | (amended 2026-10-19 from DEVOPS: identity regex uses the exact case `gizawNahom`, asserted against the first rc's real certificate; syft v1.54.1 from a sha256-pinned tarball) Keyless cosign `--recursive` over index and platforms (verify works on the tag); per-platform SPDX-JSON SBOM via syft attached with `cosign attest --type spdxjson` (ADR-021) | Matches `cosign verify`/`verify-attestation` stories; SBOM content is per platform |
| DDD-39 | Supported minimum privilege set for the migrate role as an explicit checklist (database CONNECT-grant right, CREATE on `public`, CREATEROLE, PG16 ADMIN OPTION, ownership of altered tables, cluster and collision rules), gathered as facts plus always-rolled-back empirical probes, mapped 42501 backstop (ADR-022) | Failure before any change, naming the privilege; PG16 `CREATEROLE` semantics |
| DDD-40 | Rotation is migrate-then-restart; hot reload recorded, not built (ADR-023) | Pooled connections keep the old credential only until recycled; no story asks for hot reload |
| DDD-41 | One ledgerops database per Postgres cluster (user decision): `migrate` against a target with no ledgerops schema refuses, before any change, when role `ledgerops_app` already exists in the cluster; no override flag; atomic only through the role claim of DDD-48 (ADR-017) | The role is cluster-global: Rule A holds by construction and a rotation can never reach another database |
| DDD-42 | Migration is a pure `PlanMigration(facts, config)` returning a refusal or an ordered plan; facts come from read-only catalog queries plus rolled-back empirical probes; driven ports split into `SchemaMigrator` and `RolePasswordSetter` (takes a finished verifier only) (ADR-022) | Every refusal reason is table-testable; the password setter cannot touch schema and never sees plaintext |
| DDD-43 | SBOM verify target is the platform digest (two-step lookup documented); no index-level SBOM; cosign 2.x, digest references only (ADR-021) | One merged SBOM would be wrong for one platform; avoids relying on an unverified `--platform` flag |
| DDD-44 | Pinning policy: every `uses:` by full commit SHA, tool versions pinned, all base images (incl. smoke `postgres:16`) by digest, automated unpinned-`uses:` check (ADR-019, ADR-020) | Supply-chain integrity of the pipeline that signs the image |
| DDD-45 | Re-run semantics (clarified 2026-10-19 from DEVOPS: "build anew" applies to a fresh run with the tag absent; "Re-run failed jobs" reuses the earlier job outputs, so the earlier digest is reused; every run prints `image_digest=... source=built\|reused-output\|resumed-from-tag`): if the exact tag exists, resume with its digest (skip build), else build anew; builds not claimed reproducible; no automatic pruning in v0.1.0 (ADR-020) | Idempotent re-runs without reproducible-build effort; naive pruning would break multi-arch indexes |
| DDD-46 | Smoke harness, quickstart checker and release planner are Go programs under `scripts/<name>/` (existing `scripts/race` convention); GitHub Release body is a template naming the image reference and verify commands plus generated notes; docs and example compose are components (ADR-020) | One convention; the quickstart document is the tested artifact |
| DDD-47 | SCRAM verifier uses SASLprep (RFC 4013) normalisation, injected salt and iteration count, non-ASCII acceptance case (ADR-017) | Correct for non-ASCII passwords; deterministic unit tests |
| DDD-49 | (added 2026-10-19 from DEVOPS, ADR-020 amendment) Stale-plan guard: `classify` runs the pure planner once and passes the plan; `promote` (no checkout, `packages: write`) fails "plan stale, re-run all jobs" when GHCR's highest stable tag differs from the plan's `highest_stable`. `ci` gates every path including resume; every job's `if:` names each dependency explicitly (structural test `if_clauses_complete`); `anon-probe` and `promote` also need `sign` from slice 07. The global concurrency group keeps one running and one pending run (older pending run is cancelled by GitHub): operational mitigation only | Safety of "highest stable" with a small write-permission job; `!cancelled()` disables the implicit `success()` so dependencies must be explicit |
| DDD-50 | (added 2026-10-19 from DEVOPS, ADR-020/021 amendment) Tag namespace: GHCR returns 404 to the referrers API, so signatures and attestations are `sha256-<hex>.sig`/`.att` tags in the same package; the `internal/release` classifier's existing-versions input ignores every tag that is not strict semver or `latest`, and orphan cleanup treats `.sig`/`.att` as belonging to their subject digest | Signature tags must never be read as versions |
| DDD-51 | (added 2026-10-19 from DEVOPS) Test placement: `unit` runs `./cmd/...`, `./internal/support/...`, `./internal/release/...`; the `console` job runs the whole HTTP adapter package plus a `-tags console_embed` run with the built bundle copied to `console_dist`; structural tests (Dockerfile `FROM` digests, `--platform=$BUILDPLATFORM`, no `RUN` after the last `FROM`, `-tags console_embed`, `release.yml` needs graph, `if:` clauses, no import of `internal/adapters/postgres/prefeature` from `cmd/...`, `internal/domain`, `internal/app`) run in a `workflow-guards` job; a required `container-acceptance` job (`needs: [integration]`) runs the `@dedicated-cluster` suites | Without a home, the new pure tests and embed tests would run in no CI job |
| DDD-48 | The DDD-41 guarantee is made atomic by claiming the role: when absent, `migrate` first creates `ledgerops_app` NOLOGIN with an owner-database marker (`COMMENT ON ROLE`) in its own committed transaction; the loser of a race gets 42710, re-reads the marker and refuses with the cluster-rule message; a re-run against the marked database resumes. No advisory lock (ADR-017) | `pg_advisory_lock` keys are per-database, so migrates on different databases would not contend, and a fixed shared database may deny CONNECT; role creation is the one cluster-global atomic operation. Refusal message names the privileged remedy `DROP ROLE ledgerops_app` when the marked database no longer exists |

## Wave: DESIGN / [REF] Changed Assumptions

Refinements accepted by the user 2026-10-17. DISCUSS sections are not edited; story/AC wording that needs PO revision is listed in `design/upstream-changes.md`.

1. **Smoke ordering (digest-first).** Original (this file, D8): "then a post-push smoke test pulls the published image, migrates a fresh Postgres and checks `/health`." New: the image is pushed by digest with no version tag; the smoke pulls that exact digest from GHCR, both architectures, plus the upgrade leg; signing follows; only then is `X.Y.Z` promoted, then `X.Y` and `latest`. Rationale: a failed smoke or signature leaves an untagged digest, never a public version tag. Stricter than D8, same registry bytes.
2. **Rule A crash scenario.** Original (Gherkin scenarios): "And migrate is killed after the first schema change ... And re-running migrate with "T7!kq@9x%Zm" completes and that password works". New: true only when the kill falls between migration files. A kill mid-file leaves a dirty version; `migrate` refuses naming it (DDD-29). The role stays NOLOGIN, so Rule A holds in both cases. DISTILL needs both scenarios.
3. **Test churn scope.** Original (D6): "`docker-compose.yml`, the loadtest compose, the test helper and CI set `LEDGEROPS_APP_PASSWORD=app-secret` explicitly". New: `postgres.Migrate`/`MigrateStep` change signature (password parameter) and have 7 direct call sites in 6 files (verified by grep, corrected in review cycle 1): `tests/acceptance/ledgercore/ledger_seeding.go` (2: `Migrate`, `MigrateStep`), `ledger_observations.go`, `tests/acceptance/multitenancy/world.go`, `tests/acceptance/intertenanttransfer/world.go`, `internal/adapters/postgres/testhelper_test.go`, `internal/adapters/http/trial_balance_tenant_scope_test.go`; `steps_ledger_test.go` reaches it through `MigrateFromZero`. There are at least six independent `app-secret` literals. All pass `app-secret` explicitly; the compiler finds every site.
4. **Frozen pre-feature fixture.** New obligation: before 0001 is edited, the current 0001 text (and 0002-0006) is frozen as a test fixture so US-4 and the smoke upgrade leg can build a database exactly as the pre-feature demo stack did (role LOGIN with `app-secret`, grant on database `ledgerops`).
5. **One ledgerops database per cluster (user decision, review cycle 1).** New constraint not present in DISCUSS: `ledgerops_app` is cluster-global, so a fresh `migrate` refuses when the role already exists in the cluster (DDD-41, ADR-017). DISCUSS's "The first ledgerops database in a cluster can have any name" (US-1b) stays true for the first ledgerops database in a cluster; see `design/upstream-changes.md` item 10 for the story text that needs a qualifier.
6. **Amended 2026-10-19 from DEVOPS (`devops/upstream-changes.md`, user decisions of 2026-10-17).** Original: "Node 20 stage, Vite 5 (existing)" / "alpine:3.20 (existing)" -> `node:22-slim` and `alpine:3.24` (Alpine 3.20 left support 2026-04-01; `npm ci` fails on musl, Pact 17 needs Node 22). Original ADR-020 step 3 "build per platform ... merge into an untagged index" -> one multi-platform build. Original ADR-021 identity regex `gizawnahom` -> exact case `gizawNahom`. First image release is `v0.2.0`, not `v0.1.0`, and `classify` reads versions from GHCR, never git tags. DDD-49..51 are new; DDD-34, 35, 38, 45 are annotated in place.

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
| CI workflow | `.github/workflows/ci.yml` | EXTEND | `workflow_call`; branches-only `push:`. Amended 2026-10-19 from DEVOPS (DDD-51): `unit` gains `./cmd/...`, `./internal/support/...`, `./internal/release/...`; `console` runs the whole HTTP adapter package and a `-tags console_embed` run on Node 22; new `workflow-guards` job (structural tests) and required `container-acceptance` job (`needs: [integration]`) |
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
| Base image | `alpine:3.24` (amended 2026-10-19 from DEVOPS, was alpine:3.20); every `FROM` (and smoke `postgres:16`) pinned by digest, values by DEVOPS | MIT |
| Actions | every `uses:` pinned by full commit SHA; cosign 2.x, syft, buildx versions pinned | Apache-2.0 / MIT |
| Console build | `node:22-slim` stage and CI `console` job (amended 2026-10-19 from DEVOPS, was Node 20), Vite 5 (existing) | MIT |
| Registry | GHCR via `GITHUB_TOKEN` (D1) | n/a (service) |
| Image build | docker/buildx, docker/metadata-action | Apache-2.0 |
| Signing | cosign keyless, Sigstore Fulcio/Rekor | Apache-2.0 |
| SBOM | syft v1.54.1 from its release tarball with a pinned sha256 (amended 2026-10-19 from DEVOPS), SPDX-JSON per platform | Apache-2.0 |
| Password verifier | SCRAM-SHA-256: PBKDF2-HMAC-SHA-256 and HMAC from Go stdlib; SASLprep via `golang.org/x/text/secure/precis` (module already in `go.mod` v0.40.0 as `// indirect` via pgx; becomes a direct dependency, so `go.mod` is edited) | BSD-3 |

## Wave: DESIGN / [REF] Decisions table

| DDD | Locked? | Origin | ADR |
|---|---|---|---|
| DDD-27..29, 41, 47, 48 | Yes | OQ-2, user-accepted; DDD-41 is a user decision (review cycle 1) | 017 |
| DDD-30..32 | Yes | OQ-3 | 018 |
| DDD-33..34 | Yes | OQ-5, non-root | 019 |
| DDD-35..37, 44..46 | Yes | OQ-6, review cycle 1 | 020 (044 also 019) |
| DDD-49..51 | Yes | DEVOPS back-propagation 2026-10-19 | 020, 021, 019 |
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
- (Added 2026-10-19 from DEVOPS, DDD-51) `workflow-guards` structural tests: Dockerfile stages `FROM --platform=$BUILDPLATFORM`, `GOOS`/`GOARCH` from `TARGETOS`/`TARGETARCH`, no `RUN` after the last `FROM`; `release.yml` `needs` graph has `ci` in every job after `classify` and each `if:` names all its dependencies; `cmd/...`, `internal/domain` and `internal/app` never import `internal/adapters/postgres/prefeature`.

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
- **New scenarios that need their own cluster**: anything that creates the role, sets or rotates its password, or exercises Rule A/B (US-1a, US-1b, US-4, dirty state) mutates a cluster-global object, so it runs in a dedicated container (tag `@dedicated-cluster`), as `StartWithoutSchema` already does. The cluster-rule refusal scenario may run in any cluster that already has the role by creating an empty second database and dropping it afterwards.
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

---

## Wave: DEVOPS / [REF] Prior Wave Consultation

*Owner: nw-platform-architect (Apex). Density: lean (Tier-1 only). Configuration user-approved 2026-10-17; mutation strategy already in `CLAUDE.md`, not re-asked.*

- ✓ `docs/feature/container-image-release/feature-delta.md` § DISCUSS (Outcome KPIs K1-K7, US-1b KPI, Locked decisions D1-D15, Pre-requisites) and § DESIGN (DDD-27..48, Changed Assumptions, Open questions for DEVOPS, Test harness impact)
- ⊘ `docs/feature/container-image-release/discuss/outcome-kpis.md`, `design/*.md` other than `upstream-changes.md` (single-file layout; KPIs live in this file)
- ✓ `docs/feature/container-image-release/design/upstream-changes.md`
- ✓ `docs/feature/container-image-release/slices/slice-03-*.md`, `slice-06-*.md` (staging); other slices skimmed by title only
- ✓ `docs/product/architecture/brief.md` § Deployment shape, § Container image release
- ✓ `adr-019` (image build shape), `adr-020` (pipeline order, permissions, pinning, re-run), `adr-021` (signing, SBOM); `adr-017/018/022/023` not re-read (privilege, config and rotation semantics are DISTILL/DELIVER inputs, not platform inputs)
- ✓ `docs/product/kpi-contracts.yaml`
- ✓ `.github/workflows/ci.yml`; `nightly.yml` and `load-test.yml` (triggers, `uses:` and compose calls); `Dockerfile`; `docker-compose.yml`; `docker-compose.loadtest.yml`; `Makefile`
- ✓ Precedent: `docs/feature/multitenancy/feature-delta.md` § DEVOPS and `devops/environments.yaml`; `ledger-core-console` DEVOPS headings
- ✓ Live facts gathered 2026-10-17 (not assumed): git tags and GitHub Releases of `gizawNahom/ledgerops`, recent `ci.yml` job timings, GHCR referrers behaviour, Alpine release calendar, `gh release create` help and source, registry digests, action tag SHAs

**Contradiction check.** No DEVOPS decision contradicts DESIGN's architecture. Places where DESIGN's assumptions meet reality are recorded in § Changed Assumptions and `devops/upstream-changes.md`. The four user decisions of 2026-10-17 (first release `v0.2.0`, exact-case identity regex, `alpine:3.24`, Node 22 slim) are recorded as OPS-17, OPS-24, OPS-25; revision cycle 1 (platform review) is applied in OPS-14, 16, 19, 21-23 and 27-30.

---

## Wave: DEVOPS / [REF] OPS decisions

OPS-1..9 are inherited unchanged (same numbering as `multitenancy`). New ids start at OPS-14 because 10-13 are already taken by earlier features with different meanings (ledger-core OPS-10 two-role model, referenced by D4; multitenancy OPS-10..13).

| ID | Decision | Rationale |
|---|---|---|
| OPS-1 | No hosted environment; clients self-host the published GHCR image | Inherited. K1-K7 are CI/release-gate checks, not production telemetry |
| OPS-2 | Docker Compose for demo and CI; clients use `docker run`, compose or their own Kubernetes (no manifests shipped, out of scope) | Inherited; `examples/published-image/` is the client-facing compose |
| OPS-3 | GitHub Actions | Inherited |
| OPS-4 | Brownfield: existing CI only (`ci.yml`, `nightly.yml`, `load-test.yml`); `release.yml` is the one new workflow | No existing release workflow; justification already in DESIGN Reuse Analysis |
| OPS-5 | Observability unchanged: slog JSON + `/metrics` + Prometheus/Grafana scaffold | No hosted environment to observe |
| OPS-6 | Recreate; rollback = redeploy the previous tag (by digest); migrate-then-restart (ADR-023) | Inherited; see § Deployment strategy for the rollback contract and the release-side failure matrix |
| OPS-7 | No continuous learning | Inherited |
| OPS-8 | Trunk-based development plus strict `vX.Y.Z` release tags | Inherited and extended: tags are the only publish trigger |
| OPS-9 | nightly-delta mutation testing | Already in `CLAUDE.md`; not re-asked. Scope tweak in OPS-26 |
| OPS-14 | `release.yml` shape: jobs `classify`, `ci`, `build`, `smoke`, (`sign`), `anon-probe`, `promote`, `github-release`, `quickstart`, per-job permissions as ADR-020 item 6. **One multi-platform `docker buildx build` pushes the untagged index by digest; there is no per-platform build matrix and no merge job.** Provenance and BuildKit SBOM attestations off (`provenance: false`, `sbom: false`) so the index holds exactly the declared platforms. **`ci` is in the `needs` of every job after `classify`, including on the resume path** (revision M1) | The Dockerfile cross-compiles on `$BUILDPLATFORM` and its final stage has no `RUN` (DDD-34; asserted in `workflow-guards`, M3), so nothing executes on arm64 at build time. A resumed run skips `build` but must still pass the CI gate |
| OPS-15 | arm64 smoke runs natively on `ubuntu-24.04-arm` (free for public repos; this repo is public, verified). QEMU is not used anywhere. Staging: slice 03 ships `platforms: linux/amd64` and a one-row smoke matrix; slice 06 adds `linux/arm64` and the second matrix row, K7 and the label check | Respects the DISCUSS slicing (03 is amd64-only). Slice 06 becomes a two-line change plus assertions. Fallback if the repo ever goes private: arm runner is paid, so use QEMU for the smoke only (documented, not built) |
| OPS-16 | Pinning policy resolved with real values (§ Supply-chain pins). All four existing workflows are pinned; enforced by a zizmor `unpinned-uses` hash-pin audit in `ci.yml` job `workflow-guards`; Dependabot for `github-actions`, `docker`, `gomod`, `npm`. Downloaded tool binaries are pinned by sha256 and verified in the workflow, or the installer's own verification is cited with evidence | `ci.yml` runs inside the signing run. Pins keep the currently used major of each action; Dependabot proposes majors |
| OPS-17 | **User-decided 2026-10-17:** console stage and CI console job use **Node 22 slim** (`node:22-slim`, glibc); runtime base is **`alpine:3.24`** | `npm ci` fails on `node:20-alpine` (glibc-only Pact native, reproduced); Pact 17 declares `node >=22`. Alpine 3.20 ended support 2026-04-01 |
| OPS-18 | K1-K7, the US-1b KPI and US-2's KPI are wired into `docs/product/kpi-contracts.yaml` as `KPI-R1..R9`, all `class: gate`, emitted as one summary table per release run; KPI-5 is re-baselined | See § Monitoring contracts |
| OPS-19 | Signature-storage check in the `sign` job (tag-fallback vs OCI referrers), plus anonymous `cosign verify` and `verify-attestation` in the `quickstart` job; the release classifier must ignore `sha256-<hex>.sig/.att/.sbom` tags; a GHCR tag-list 404 on the first run (package does not exist yet) is the empty set, with a test | GHCR answered 404 to the referrers endpoint for a real public digest on 2026-10-17, so tag-fallback is expected, not assumed |
| OPS-20 | GitHub Release: `gh release create "$TAG" --verify-tag --title … --notes-file notes.md --generate-notes [--prerelease] --latest=<moveLatest>`; skipped if the release already exists (then only the body assertion runs) | Confirmed from `gh release create --help` and `pkg/cmd/release/create/create.go` (v2.102.0): `-F` fills the same `Body` as `--notes`, which `--generate-notes` prepends |
| OPS-21 | No automated pruning of orphaned digests in v0.1.0; manual, dry-run-first runbook (reachable set per revision B1, age window 35 days) in § Deployment strategy | DDD-45 |
| OPS-22 | First-release owner runbook (OQ-7). **The `v*` tag ruleset is a required step before the first rc** (revision m10); immutable releases optional | One-time; the failed-by-design anonymous probe is documented, not fixed |
| OPS-23 | `ci.yml` changes: `workflow_call`; `push` restricted to branches; top-level `permissions: contents: read`; `persist-credentials: false` on every checkout; Node 22 in the console job; `unit` job gains `./internal/release/... ./internal/support/... ./cmd/...`; console job runs the whole `internal/adapters/http` package (plus a `console_embed` build run); new jobs `workflow-guards` and **`container-acceptance`** (OPS-28). Compose `migrate` service becomes `build`/shared-image plus `command: [migrate]` | See § CI/CD pipeline outline. Pre-existing gap found: apart from the Pact test, `internal/adapters/http` tests run in no CI job |
| OPS-24 | **User-decided 2026-10-17:** the published cosign identity regex uses the exact case `gizawNahom`, as in the certificate: `^https://github.com/gizawNahom/ledgerops/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+(-.+)?$` with issuer `https://token.actions.githubusercontent.com`. The first rc's `sign` job prints the real certificate identity and asserts the published regex matches it (no mock). The image reference stays lowercase (`ghcr.io/gizawnahom/ledgerops`) | GHCR requires lowercase; the certificate carries the repository path as GitHub reports it |
| OPS-25 | **User-decided 2026-10-17:** the first image release is **`v0.2.0`** (rcs `v0.2.0-rc.N`). The existing `v0.1.0` tag and pre-release are left untouched. `classify` takes existing versions from GHCR, not git tags. D14 amendment goes upstream | `v0.1.0` is a public source-only pre-release on a commit without `release.yml` |
| OPS-26 | Mutation scope: `nightly.yml`'s `mutation-delta` file filter widens from `^internal/domain/` to `^internal/(domain\|release)/`. `PlanMigration` and the SCRAM builder live beside Testcontainers tests in `internal/adapters/postgres`, so they are not added (each mutant would run the database suite) | `internal/release` is pure and holds the `latest`/backport rule, the feature's most mutation-worthy logic. Strategy itself unchanged (nightly-delta) |
| OPS-27 | Concurrency: keep one global group, no cancel-in-progress (`group: release`). GitHub holds one running and one pending run per group and **cancels an older pending run when a newer one queues**. Mitigation: `classify` (needs `actions: read`) emits a notice listing other queued or in-progress release runs; the runbook says never push two release tags within one release duration (about 20 minutes) and, after any cancelled release run, re-run it (or re-push if the tag was never released) and confirm with `docker buildx imagetools inspect <repo>:X.Y.Z`. Rejected: a per-tag group (two runs would then promote concurrently, and `latest` could move twice out of order); a job-level group on `promote` only (same one-pending rule) | Serialisation of "highest stable" is the reason for the group (ADR-020 item 5); the cost is a rare cancelled run, which is visible and re-runnable |
| OPS-28 | New required `ci.yml` job **`container-acceptance`** (`needs: [integration]`) runs the `@dedicated-cluster` suites: US-1a, US-1b, US-2, US-4, dirty-state, SCRAM, `ddl-logging-cluster`, privilege fixtures, console-embed build tests. Suite path is DISTILL's. It is the emitter named by KPI-R3, R4, R8, R9 | These scenarios mutate cluster-global objects and need their own containers, so they must not share the `integration` job's package run; wall-clock expected to be the longest after `inter-tenant-transfer`, so it runs in parallel with it |
| OPS-29 | `promote` runs no repository code and no checkout. `classify` runs `scripts/releaseplan` once and passes the plan as job outputs. Because "Re-run failed jobs" can replay a stale plan, `promote` guards `latest` with a shell check that the current highest stable GHCR tag equals the plan's `highest_stable`; otherwise it fails with "plan stale, re-run all jobs". `highest_stable` is the empty string on the first release (no stable tag exists); the guard compares empty to empty. Before any tag exists `promote` proves that single-source `imagetools create` keeps the index digest (§ Deployment strategy) | Keeps the `packages: write` job minimal. This replaces ADR-020's "re-evaluate `moveLatest` at that moment" with an equivalent fail-safe; upstream item 9 |
| OPS-30 | Re-run semantics reconciled with ADR-020 item 4/11 (§ Deployment strategy): the contract is the ADR's (tag exists: resume with that digest; tag absent: a fresh run builds anew). "Re-run failed jobs" reuse of the same run's earlier outputs is a GitHub-level optimisation, not a contract. Every run prints `image_digest=<sha256> source=built\|reused-output\|resumed-from-tag` in the job summary | One unambiguous rule for the runbook and the orphan cleanup |

---

## Wave: DEVOPS / [REF] Environment matrix

Machine artifact: `docs/feature/container-image-release/devops/environments.yaml` (extends `multitenancy`'s, which extends `ledger-core`'s). Inherited and unchanged: `clean` (now builds the console, so slower), `ci`, `contended`, `corrupted`, `populated`, `load`.

| Environment | Platform | Preconditions | Exercises |
|---|---|---|---|
| `fresh-pg16-cluster` | linux · amd64 · arm64 | empty cluster, no `ledgerops_app` role; any database name; Postgres 16 (digest-pinned in smoke) | US-1a, K3, smoke fresh leg |
| `legacy-demo-db` | linux · amd64 | cluster whose single database was built by the **frozen pre-feature migrations 0001-0006** (role LOGIN with `app-secret`, `GRANT CONNECT` on database `ledgerops`), with entries and a zero trial balance; variants at schema version 4 and 6 | US-4, K4, smoke upgrade leg |
| `role-exists-other-db` | linux | cluster where `ledgerops_app` already exists for another database; variants: owner database present, owner database dropped (orphan marker), abandoned first run | US-1b, K3 family (KPI-R8), concurrency scenario |
| `managed-pg-nonsuperuser` | linux | privileged role is **not** superuser. Fixtures: has the minimum set (CREATEROLE, ADMIN OPTION on the app role, database owner, CREATE on `public`); and one variant per missing privilege (no CREATEROLE, no ADMIN OPTION, not database owner, no CREATE on `public`, table owned by another role, no CONNECT-grant right) | ADR-022 probe, "no changes applied" |
| `image-amd64` / `image-arm64` | linux/amd64 on `ubuntu-24.04`; linux/arm64 on `ubuntu-24.04-arm` (slice 06) | image pulled **by digest** from GHCR; native execution | smoke per arch, K7 |
| `nonroot-secret-files` | linux | image user 65532:65532; password file mode 0444/0644 (works), 0400 owned by root (named error), CRLF and LF trailing newline | US-1a secret-file scenarios |
| `ddl-logging-cluster` | linux | cluster started with `log_statement=all`; migrate run with a sentinel password | server log never contains the plaintext (SCRAM verifier only) |
| `dirty-migration` | linux | backend terminated mid-file (`pg_terminate_backend`) leaving `dirty = true`; and killed between files | DDD-29 |
| `ghcr-private-package` / `ghcr-public-package` | GitHub Actions | first push (package private, anonymous manifest request returns 401/404) vs after owner flips visibility | `anon-probe`, OQ-7 runbook |
| `release-ci` | GitHub Actions `ubuntu-24.04`, `ubuntu-24.04-arm` | `packages: write` / `id-token: write` only on the jobs that need them; OIDC available | `release.yml` end to end |
| `serve-config` | linux · amd64 | image started with each combination of missing, blank and whitespace `LEDGEROPS_OPERATOR_KEY`/`LEDGEROPS_APP_DSN`; demo key unset, blank, set; `LEDGEROPS_MIGRATE_DSN` present; sentinel key and DSN values on every failure path | US-2, KPI-R9, OPS-10 enforcement |
| `wrong-password-start` | linux · amd64 | database migrated with password A, app DSN carries password B | named startup refusal (SQLSTATE 28P01), neither password in the log |
| `console-embed-build` | linux · amd64 | variants: `console_dist` absent or empty (build must fail), directory without `index.html` (binary refuses to start), untagged build (serves from `consoleDistDir` as before) | US-5, ADR-019 |
| `scram-unicode` | linux · amd64 | passwords in composed, decomposed and fullwidth forms; real pgx login after the set | DDD-47 |
| `same-db-concurrent` | linux · amd64 | two `migrate` runs against the same empty database started together | loser resumes via the marker, both end consistent |
| `rule-b-interrupted` | linux · amd64 | legacy database where the migrate run fails or is interrupted before the password step | `app-secret` still works and the output says the password was not changed |
| `resume-from-tag` | GitHub Actions | GHCR already holds `X.Y.Z` (partial promote, complete release, or a different digest); first release has `highest_stable = ""` and a 404 tag list | resume path, `ci` still runs, stale-plan guard, moved-tag refusal |

---

## Wave: DEVOPS / [REF] CI/CD pipeline outline

### Branch and trigger alignment (OPS-8)

| Event | Workflow | What runs |
|---|---|---|
| push to any branch | `ci.yml` | full gate suite (unchanged), via `on.push.branches: ['**']` |
| pull_request | `ci.yml` | same |
| push of tag `vX.Y.Z[-pre]` | `release.yml` only | `ci.yml` as a called gate, then the release pipeline. `ci.yml`'s own `push` no longer fires for tags (no duplicate run) |
| `workflow_call` from `release.yml` | `ci.yml` | same jobs; the called run inherits the caller's permissions ceiling (`contents: read`) |
| schedule / dispatch | `nightly.yml`, `load-test.yml` | unchanged |

Releases are cut from `main`. A backport (`v0.1.9` after `0.2.0`) needs a short-lived `release/X.Y` branch; `ci.yml` already runs on every branch. `workflow_dispatch` is deliberately **not** a release trigger: a certificate minted from a branch ref (`@refs/heads/...`) would not match the published identity, so only tag runs can produce verifiable images.

### `release.yml` outline (structure, not YAML)

Workflow-level: `on.push.tags: ['v[0-9]+.[0-9]+.[0-9]+', 'v[0-9]+.[0-9]+.[0-9]+-*']` (re-validated strictly in `classify`); `permissions: contents: read`; `concurrency: {group: release, cancel-in-progress: false}` (OPS-27); runner `ubuntu-24.04` unless stated; every `uses:` by SHA (§ pins); every `actions/checkout` with `persist-credentials: false`; every `run:` reads the tag through `env: TAG: ${{ github.ref_name }}` and `"$TAG"`, never an inline expression (script injection). Path `.github/workflows/release.yml` and in-file signing are frozen (ADR-020 item 7).

**Gating rule (M1, X1).** `ci` is in the `needs` of every job after `classify`. A status function in an `if:` (`!cancelled()`) switches off the implicit `success()`, so **each job below carries an explicit `if:` of the form `!cancelled() && needs.<dep>.result == 'success'` for every dependency that must succeed**; nothing is inherited. `build` is skipped on the resume path, so only jobs that accept a skipped build use `(needs.build.result == 'success' || needs.build.result == 'skipped')` for `build`. The digest comes from `needs.build.outputs.digest || needs.classify.outputs.resume_digest`. A resumed release therefore still passes the full suite, and a failed smoke, probe, sign or promote stops everything after it.

| Job | `if:` (all clauses `&&`-joined, after `!cancelled()`) |
|---|---|
| `ci` | `needs.classify.result == 'success'` |
| `build` | `needs.classify.result == 'success' && needs.ci.result == 'success' && needs.classify.outputs.resumed != 'true'` |
| `smoke` | `classify`, `ci` success; `build` success or skipped |
| `sign` (slice 07) | `classify`, `ci` success; `build` success or skipped; `smoke` success |
| `anon-probe` | `classify`, `ci` success; `build` success or skipped; `smoke` success; **`sign` success from slice 07** |
| `promote` | `classify`, `ci` success; `build` success or skipped; `smoke` success; `anon-probe` success; **`sign` success from slice 07** |
| `github-release` | `classify`, `ci`, `promote` success |
| `quickstart` | `classify`, `ci`, `github-release` success |

`workflow-guards` parses each job's `needs` and `if:` in `release.yml` and asserts: `ci` is in every downstream job's `needs`; every dependency listed in the table appears both in `needs` and as a `needs.<dep>.result == 'success'` clause (the `build` dependency may instead carry the success-or-skipped clause, and only on `smoke`, `sign`, `anon-probe`, `promote`); each job's `if:` begins with `!cancelled()`; no job has an `if:` that lacks a clause for one of its dependencies. KPI-R2 gains the field `if_clauses_complete`.



| # | Job | Needs | Permissions (beyond `contents: read`) | Does | Failure leaves |
|---|---|---|---|---|---|
| 1 | `classify` | none | `packages: read`, `actions: read` | validate tag against strict semver; list existing GHCR tags (a 404 for the unknown package on the first run is the empty set, tested; `sha256-*` signature tags ignored); run `scripts/releaseplan` **once** and emit the plan as outputs (version, tags, prerelease, `move_latest`, `highest_stable`, `resumed`, `resume_digest`); notice listing other queued or in-progress release runs (OPS-27) | nothing |
| 2 | `ci` | classify | inherits `contents: read` | `uses: ./.github/workflows/ci.yml` (includes `container-acceptance`, `workflow-guards`) | nothing |
| 3 | `build` | classify, ci | `packages: write` | skipped when `resumed`. buildx (pinned, sha256-verified), `docker/login-action`, `metadata-action` for labels, `build-push-action` with `outputs: type=image,name=…,push-by-digest=true,name-canonical=true,push=true`, `platforms` per slice, build args `VERSION`/`COMMIT`, `provenance: false`, npm cache mount. Summary prints `image_digest=… source=built` | untagged digest |
| 4 | `smoke` (matrix: amd64 on `ubuntu-24.04`; arm64 on `ubuntu-24.04-arm` from slice 06) | classify, ci, build | `packages: read` | pull **by digest**; `go run ./scripts/smoke` (Postgres from Testcontainers, image ref read from the example compose, § Smoke inputs): fresh leg and upgrade leg; timing and results to `$GITHUB_STEP_SUMMARY` | untagged digest |
| 5 | `sign` (slice 07) | classify, ci, build, smoke | `packages: write`, `id-token: write` | cosign v2.6.5 (binary sha256 verified), syft tarball (sha256 verified); `cosign sign --yes --recursive` on the index digest; per platform digest: syft SPDX-JSON then `cosign attest --yes --type spdxjson`; verify index and each platform with the exact-case regex (prints the real certificate identity and asserts the match); `verify-attestation` per platform digest; signature-storage check | untagged digest, signature artifacts |
| 6 | `anon-probe` | classify, ci, build, smoke; **plus `sign` from slice 07** | `{}` (no token, no login) | unauthenticated bearer-token request to ghcr.io, `GET /v2/gizawnahom/ledgerops/manifests/<digest>` must answer 200; on 401/404 fail with the OQ-7 runbook message | untagged digest |
| 7 | `promote` | classify, ci, build, smoke, anon-probe (, sign) | `packages: write` | **no checkout, no repository code.** `setup-buildx-action` (same pinned SHA, `version: v0.38.0`) followed by the same sha256 assertion on the installed plugin as in `build` (§ Supply-chain pins), because `promote` uses `imagetools`; stale-plan guard for `latest` (OPS-29); pre-tag digest-preservation check; guard `X.Y.Z` absent or already this digest; `docker buildx imagetools create -t …:X.Y.Z <repo>@<digest>`; then `X.Y` and `latest` per the plan (slice 06); post-condition: each tag resolves to the digest | `X.Y.Z` public; re-run resumes the rest |
| 8 | `github-release` (slice 06) | classify, ci, promote | `contents: write` | checkout for the template; render `notes.md` with `envsubst` from `.github/release-notes.md.tmpl` (golden-file test in `unit`); skip creation if the release exists; else `gh release create` (§ Deployment strategy); assert the body contains the image reference | tags public, no release page; re-run completes |
| 9 | `quickstart` (slice 08) | classify, ci, github-release | `contents: read` (checkout only); **no packages permission, no login** | `go run ./scripts/quickstart` against `:X.Y.Z`, including anonymous `cosign verify` and `verify-attestation` | release public; run red; fix docs, re-run |

Every job echoes `image_digest=<sha256> source=…` to **stdout (the job log) and** the job summary, so a human can see which digest a run acted on and the cleanup can grep it.

Slice staging: 03 = jobs 1-4, 6, 7 (exact tag only, amd64). 05 adds `/console` and `version` assertions to 4. 06 adds arm64, `X.Y`/`latest`, labels, job 8, K7. 07 inserts job 5 and adds it to the `needs` of 6 and 7. 08 adds job 9.

**Image topology (M3, consistent with ADR-019 and DDD-34).** The Go stage and the Node stage are `FROM --platform=$BUILDPLATFORM …`; `go build` takes `GOOS=$TARGETOS GOARCH=$TARGETARCH` with `CGO_ENABLED=0`; the final stage is `FROM alpine:3.24@sha256:…` with **no `RUN`** (the CA bundle is copied from the build stage, DELIVER confirms the file exists there or adds `apk add ca-certificates` to the build stage). Result: arm64 is cross-compiled on the amd64 runner, nothing executes under emulation, and only the smoke needs a native arm64 runner. `workflow-guards` asserts all three properties by parsing the Dockerfile: both builder stages carry `--platform=$BUILDPLATFORM`, `GOOS`/`GOARCH` come from `TARGETOS`/`TARGETARCH`, and no `RUN` follows the last `FROM`.

**Smoke inputs (m9).** The smoke starts Postgres through Testcontainers on the runner's Docker (no compose), image ref read from `examples/published-image/docker-compose.yml` (digest-pinned, tracked by Dependabot). The frozen pre-feature migrations 0001-0006 live as an embeddable package `internal/adapters/postgres/prefeature/` (`//go:embed *.sql`, importable by `scripts/smoke` and the acceptance suites, since `internal/` is module-wide), protected by the same checksum guard as the live migrations. The upgrade leg applies them as superuser, seeds entries through SQL, then runs the image's `migrate`.

Quality-gate map (shift-left): local = `go test ./...`, `go run ./scripts/releaseplan --dry-run vX.Y.Z`, `go run ./scripts/smoke --image <local>` (pre-commit not adopted, inherited) → PR = existing required jobs plus `workflow-guards` and `container-acceptance` → CI = `ci.yml` as the release gate on every path → deploy = digest-first order with the anonymous probe as the promotion gate → post-release = `quickstart`. Thresholds (120 s smoke, 600 s quickstart) live in `kpi-contracts.yaml`.

### `ci.yml` changes (item 9)

| Change | Why |
|---|---|
| `on: {push: {branches: ['**']}, pull_request: {}, workflow_call: {}}` | no duplicate run on tag push; `release.yml` can call it. `'**'` (not `'*'`) so branch names with slashes still match |
| top-level `permissions: contents: read` | none today (repo default applies); the called run is capped by the caller anyway |
| every `uses:` by SHA | OPS-16 |
| `console` job: `node-version: "22"` | OPS-17 |
| `unit`: add `./internal/release/... ./internal/support/... ./cmd/...` (pure; `-race`) | new pure code (classifier, `Secret`, config parsers) has no CI home today |
| `console`: run `go test ./internal/adapters/http/... -count=1` (replaces the `-run TestPactProvider…` filter; pact lib already installed in that job) and, after `Vite build`, copy `dist` to `internal/adapters/http/console_dist` and run once more with `-tags console_embed` | embed seam and no-`index.html` tests need a CI home; the package's other tests run in no CI job today (five test files; `-run` filter; pre-existing gap) |
| new job `workflow-guards` (no DB, no `needs`) | zizmor with `.github/zizmor.yml` (`unpinned-uses: policies: {"*": hash-pin}`) over all workflows; `FROM` lines carry `@sha256:`; Dockerfile build line carries `-tags console_embed`; Dockerfile topology (M3); an import guard (depguard rule or AST test) that `cmd/...`, `internal/domain/...` and `internal/app/...` never import `internal/adapters/postgres/prefeature`, so the frozen pre-feature SQL cannot reach the serving binary; **structural test of `release.yml`'s `needs` graph (K2): every job after `classify` needs `ci`; `smoke`, `anon-probe`, `promote` need `build`'s digest source; `promote` needs `smoke` and `anon-probe`; `anon-probe` and `promote` need `sign` once slice 07 lands**. Required check |
| new job `container-acceptance` (`needs: [integration]`) | OPS-28; required check; emitter for KPI-R3, R4, R8, R9 |
| `demo` job: add a step after the timing step | emits `::warning title=KPI-5::demo_first_green_seconds=<n> over 200` when above 200 (the KPI-5 guardrail emitter); never fails the job |
| every `actions/checkout` gets `persist-credentials: false` | no job here pushes with the checkout token |
| `demo` job unchanged in shape | it already builds the Dockerfile (so it is the PR-time image-build gate, incl. the Node stage); no separate image-build job (simplest alternative: rejected a duplicate build job) |

Compose consequences (`migrate` service now `build:` the repo Dockerfile with `command: ["migrate"]`):
- env `LEDGEROPS_MIGRATE_DSN` (the existing `ledgerops_migrate` bootstrap superuser, so the privilege preflight passes) and `LEDGEROPS_APP_PASSWORD: app-secret` on `migrate` only. **`app` must never receive `LEDGEROPS_MIGRATE_DSN`**: no shared `env_file`, no YAML anchor merged into both (serve refuses and the demo would fail to start).
- give `migrate` and `app` the same `image:` tag (for example `ledgerops:dev`) so the second build is a layer-cache hit rather than a second Node build.
- the `./internal/adapters/postgres/migrations:/migrations:ro` bind mount and the `migrate/migrate:v4.18.3` image are removed; nothing else in the repo references that image (`go.mod` keeps the library).
- `depends_on: service_completed_successfully` is unchanged. `load-test.yml` calls `docker compose up -d postgres migrate app …`, still valid; `docker-compose.loadtest.yml` overrides only `app`/`postgres` and keeps `app-secret` in the DSN, still valid.
- The image runs as 65532; `migrate` has no volume or secret file in compose, so the `0400 root` trap does not apply to the demo.
- `Makefile` `up` (`docker compose up -d --build`) is unchanged but slower (KPI-5). The old binary name `ledgerops-api` appears only in the `Dockerfile`.

---

## Wave: DEVOPS / [REF] Supply-chain pins

Resolved 2026-10-17 against the real upstreams; none invented. Re-resolve any row with the command shown (`git ls-remote` for actions, `docker buildx imagetools inspect <ref> --format '{{.Manifest.Digest}}'` for images). Tags are lightweight unless noted, so the SHA is the tag target today; the pin is what makes them stable. I did not check security advisories for these versions; Dependabot alerts cover that after the file lands.

**Actions** (`uses: owner/repo@<sha> # <tag>`); `git ls-remote --tags https://github.com/<owner>/<repo>.git '<tag>*'`:

| Action | Tag | Commit SHA | Note |
|---|---|---|---|
| actions/checkout | v4.4.0 | `11d5960a326750d5838078e36cf38b85af677262` | highest v4 |
| actions/setup-go | v5.6.0 | `40f1582b2485089dde7abd97c1529aa768e1baff` | highest v5 |
| actions/setup-node | v4.4.0 | `49933ea5288caeca8642d1e84afbd3f7d6820020` | highest v4 |
| actions/upload-artifact | v4.6.2 | `ea165f8d65b6e75b540449e92b4886f43607fa02` | highest v4 |
| golangci/golangci-lint-action | v7.0.1 | `9fae48acfc02a90574d7c304a1758ef9895495fa` | highest v7 |
| grafana/setup-k6-action | v1.2.2 | `43b9fc21641a76002687994433dd586f56e791b1` | highest v1 (`load-test.yml`) |
| docker/setup-buildx-action | v4.4.1 | `f87e5991a6d7451dcb8d9637bfbc97413f497069` | new |
| docker/login-action | v4.6.0 | `dbcb813823bdd20940b903addbd779551569679f` | new |
| docker/build-push-action | v7.4.0 | `c3c9e263c25d99ce0380d002d59b67737d91b0dc` | new |
| docker/metadata-action | v6.2.0 | `dc802804100637a589fabce1cb79ff13a1411302` | new; labels only |
| sigstore/cosign-installer | v4.1.2 | `6f9f17788090df1f26f669e9d70d6ae9567deba6` | new; **set `cosign-release: v2.6.5`** (its default is v3.0.6) |
| zizmorcore/zizmor-action | v0.7.1 | `85b2b9fb8e3325e7a23d7d0a9d7ba5ae0f999e3e` | new; `version: 1.31.1`, `advanced-security: false`, `annotations: true` |

Dropped from the design: `anchore/sbom-action`. Syft is fetched directly (one fewer third-party action in the signing job): `https://github.com/anchore/syft/releases/download/v1.54.1/syft_1.54.1_linux_amd64.tar.gz`, `sha256 c069905b391cc4c20a5ba65ad5c10be2a7ba074f8ea6ad203e24d14e303dad47` (checked against the release's `syft_1.54.1_checksums.txt`; extracted, `syft version` reports 1.54.1). Not used: `docker/setup-qemu-action`. `gh` is preinstalled on the runner and is pinned by the runner label `ubuntu-24.04`.

**Tool versions and artifact checksums (M5).** Dependabot cannot bump these; they go on the release-prep checklist. Each is verified in the workflow, not trusted:

| Tool | Version | Pin | Verification |
|---|---|---|---|
| cosign | v2.6.5 (source commit `3e82f50a2839855693aacf7b3d0e7e2f30774cb4`, annotated tag) | `cosign-release: v2.6.5` on the installer; linux-amd64 binary sha256 `c3b4f5410e608af03a5eb0aaac84a4313d8da131248e08ff1759ac70c79d1644` (arm64 `426193b4c5da4d4d643e822f48fe0cc8a476ca1782a272704831f5a0cef716d7`), from the release's `cosign_checksums.txt` | the installer verifies the requested version with a bootstrap cosign whose sha256 is embedded in the pinned action (read in its `action.yml`); the workflow additionally runs `sha256sum` on `$(which cosign)` against the value above |
| buildx | v0.38.0 (source commit `0dbe87f36ed472fe36b6796c3f70b4210a731fc5`) | `version: v0.38.0` on `setup-buildx-action`; linux-amd64 sha256 `4fe4cc38adf48169132749b6ca22a990928db0118e3407584ee553723115d287` (arm64 `38e890e1a162bfdbf32fbe91404991dc4fc28e55b0de901b34374a061bab4d2c`) from the release's `checksums.txt` | workflow compares `sha256sum` of the installed `docker-buildx` plugin to the value above; the build job fails on mismatch |
| syft | v1.54.1 (commit `b254e6d92f28c3868a755f62fb3ca8f26e9fee76`) | tarball sha256 above | `sha256sum -c` before extraction |
| zizmor | 1.31.1 (commit `effdb35fa7868863f11655db256d9008922fa427`) | `version: 1.31.1` on the action | the pinned `zizmor-action`'s `action.sh` installs "from a hash-verified wheel" with `pip --require-hashes` (header comment and lines 88-100, read at the pinned SHA). For reference the linux x86_64 tarball sha256 is `d185f726bdaf92df78273217dfdd24ebcd6b0ef48cf3ce776ff5762b6e138c99` |

ADR-021 asks for cosign 2.x; v3.1.3 exists and changes defaults, so moving to it is an ADR amendment, not a bump. The installer binaries' checksums were taken from the upstream release files on 2026-10-17 over HTTPS; they establish "the binary is the one released", not independent provenance. `ca-certificates` is handled by the image topology (M3), not by an `apk` step in the final stage.

**Base images** (index digests; every one lists linux/amd64 and linux/arm64):

| Stage / use | Reference | Digest | Note |
|---|---|---|---|
| Go build | `golang:1.25-alpine` | `sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59` | go1.25.14; runs on `$BUILDPLATFORM` |
| Console build, **user-decided** | `node:22-slim` | `sha256:c3de60bf2f9dd0ac6370e6117950ff62d6e339527e7472301c9c78a017978392` | **recommended**; glibc. Measured locally: `npm ci` 61 s cold, `vite build` 3 s, `dist` 172 KB |
| Console build (not usable) | `node:20-alpine` | `sha256:fb4cd12c85ee03686f6af5362a0b0d56d50c58a04632e6c0fb8363f609372293` | `npm ci` fails (`EBADPLATFORM`, glibc-only Pact native). Do not use |
| Console build (fallback) | `node:20-slim` | `sha256:2cf067cfed83d5ea958367df9f966191a942351a2df77d6f0193e162b5febfc0` | works but Node 20 is past its support window and Pact 17 wants `>=22` |
| Runtime, as designed (superseded by user decision) | `alpine:3.20` | `sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc` | 3.20.10; **branch end of support 2026-04-01** (alpinelinux.org releases.json) |
| Runtime, **user-decided** | `alpine:3.24` | `sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6` | support to 2028-06-01. Alternatives: 3.23 `sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0` (to 2027-11-01), 3.22 `sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8` |
| Smoke and example compose | `postgres:16` | `sha256:ca0bd484cb98bf4b24eb1010e73fb3fcbd6714d240fbc1a10eea5b7dbecb641d` | 16.15 (Debian). The smoke harness reads this one pin from `examples/published-image/docker-compose.yml`, which Dependabot's docker ecosystem tracks (it cannot see a string inside Go code) |

The test suites still use floating `postgres:16` (six files). `multitenancy`'s `environments.yaml` says "pinned by digest" for them; that was never true. Left alone (out of scope, no release impact); note in § Open questions.

**Automated unpinned-`uses:` check (DDD-44).** zizmor, because it also audits template injection and excessive permissions in the workflow that signs the image. `.github/zizmor.yml`: `rules: {unpinned-uses: {config: {policies: {"*": hash-pin}}}}`. Runs in `workflow-guards` on every PR and push, so `release.yml` is checked before a tag is ever cut. Local `uses: ./…` are exempt by zizmor's own rule. Fallback if zizmor proves noisy on `nightly.yml`/`load-test.yml`: restrict its inputs to `release.yml` and `ci.yml` and add a three-line grep test for the rest.

**Dependabot (recommended over Renovate).** Native, no app to install, and it understands SHA pins with trailing version comments. `.github/dependabot.yml`: `github-actions` on `/`, `docker` on `/` and `/examples/published-image`, `gomod` on `/`, `npm` on `/web/console`; weekly; grouped per ecosystem. Renovate's custom managers would also bump the four tool versions above, which does not justify a second system for four values.

---

## Wave: DEVOPS / [REF] Monitoring contracts

No production telemetry exists (OPS-1). Every KPI is a release-gate or CI assertion; its "dashboard" is the release run's job summary (`$GITHUB_STEP_SUMMARY`), one table per run. Full entries are in `docs/product/kpi-contracts.yaml` (`KPI-R1..R9`).

| KPI | Instrument (emitting component) | Fields | Assertion | Gate |
|---|---|---|---|---|
| K1 → KPI-R1 | `scripts/smoke` per arch; `scripts/quickstart` | `smoke_pull_to_healthy_seconds`, `quickstart_pull_to_first_transfer_seconds`, `arch` | ≤ 120 s each arch; ≤ 600 s | `smoke`, `quickstart` |
| K2 → KPI-R2 | workflow structure test + promote-job summary | `promote_needs_smoke_and_probe`, `all_jobs_need_ci`, `if_clauses_complete`, `latest_moved` | `ci` in the needs of every job after classify (resume path included); `promote` cannot start unless smoke (and sign, probe) succeeded; `latest` only when `move_latest` | `workflow-guards`, `promote` |
| K3 → KPI-R3 | acceptance (dedicated cluster) and smoke fresh leg | `app_secret_login_refused`, `observations`, `supplied_password_login_ok`, `plaintext_in_server_log` | refused == observations, ok, false | `container-acceptance`, `smoke` |
| K4 → KPI-R4 | smoke upgrade leg; upgrade acceptance | `entries_before`, `entries_after`, `imbalance_minor` | equal, equal, 0 | `smoke`, `container-acceptance` |
| K5 → KPI-R5 | `sign` and `quickstart` | `signatures_per_digest_min`, `sbom_attestations_per_platform_min`, `cert_identity`, `signature_storage` | for the index and each platform digest at least 1 signature; for each platform digest at least 1 SBOM attestation; identity matches the published exact-case regex | `sign`, `quickstart` |
| K6 → KPI-R6 | smoke (slice 05) | `console_http_status` | 200 | `smoke` |
| K7 → KPI-R7 | `promote` verification step (slice 06) | `index_platforms`, `label_version`, `label_revision`, `label_source` | exactly amd64+arm64; labels equal plan, commit, repo URL | `promote` |
| US-1b → KPI-R8 | acceptance (dedicated cluster) | `second_db_attempts`, `second_db_refusals`, `concurrent_winners`, `unchanged_universe` | refusals == attempts; winners == 1; snapshot diff empty | `container-acceptance` |
| US-2 → KPI-R9 | acceptance (process exit and message) | `missing_var_runs`, `refused_at_start`, `secrets_in_output` | equal, false | `container-acceptance` |

DORA, for orientation (estimates, not measured): lead time tag-to-public about 15 minutes (the `ci.yml` critical path is `integration` then `inter-tenant-transfer`, about 6 minutes measured on 2026-10-16 runs, plus build, smoke, probe, promote); deployment frequency is release cadence by choice; change failure rate is whatever escapes the smoke, tracked by hand until there are releases. No SLO or alert is defined: there is no service to page for.

**KPI-5 re-baseline (`demo_first_green_seconds`).** Measured on the three most recent green `main` runs (2026-10-16, run ids 38962444215, 38964268513, 38965301338): the `demo` step took 71, 70 and 68 seconds. The image build now adds the Node stage: local cold measurement `npm ci` 61 s plus `vite build` 3 s, so the expectation is about 135 s on a cold runner. BuildKit npm cache mounts do not help on a fresh CI runner (the cache starts empty each run) and only speed up local rebuilds; the second compose service sharing the image tag is a layer-cache hit. The 300 s target is unchanged and leaves about 2x headroom; a warn-only guardrail at 200 s is added, emitted by a new `demo` job step as a `::warning` annotation (KPI-5 `rebaseline.guardrail.emitted_by`). The actual figure is recorded when slice 05 lands. Rejected: a `gha` BuildKit cache for the demo job (adds cache-poisoning surface and a moving part for a gate that has headroom).

---

## Wave: DEVOPS / [REF] Deployment strategy

**Recreate; rollback is redeploying the previous tag, by digest** (OPS-6). There is no hosted environment, so this is the contract clients are told, not something the project runs. A client rolls back by stopping the app and starting the previous `X.Y.Z` (pinned by digest, as the quickstart recommends). It is safe while migrations stay expand-only (a standing constraint). Rotating the app password is independent of the image version. Migrate-then-restart (ADR-023); `migrate` never runs at app start.

Rollback comes first for the **release itself**: the digest-first order makes steps 3-6 reversible by doing nothing, since nothing public exists until `promote`.

### Re-run semantics (M4, reconciles ADR-020 items 4 and 11)

The contract is the ADR's (DDD-45): if `X.Y.Z` already exists in GHCR the run **resumes** with that tag's digest (build skipped; `ci`, smoke, signing checks and the probe still run against it); if the tag is absent, a **fresh run builds anew**. This is what "the re-run builds anew" in ADR-020 item 11 means for a fresh run (re-push of an unreleased tag, or "Re-run all jobs"). GitHub's "Re-run failed jobs" is a different mechanism: it replays the same run and reuses the outputs of jobs that already succeeded, so the earlier digest is reused without a rebuild. That is an optimisation the runbook may recommend, not something the pipeline depends on: if it does not behave so on the first run, "Re-run all jobs" builds anew and the first digest becomes an orphan (harmless, see cleanup). Every job prints `image_digest=<sha256> source=built|reused-output|resumed-from-tag`, so which digest a run acted on is never ambiguous. No ADR change is needed; upstream item 9 asks for a clarifying sentence.

| Failure at | State left | Recovery |
|---|---|---|
| `classify`, `ci`, `build` | nothing, or an untagged digest | fix, push a new tag (an unreleased tag may be moved; never after `promote` ran) |
| `smoke`, `sign`, `anon-probe` | untagged digest (and signature artifacts) | fix, then "Re-run failed jobs" (reuses the digest) or "Re-run all jobs" (builds anew, orphans the old digest) |
| `promote` partway (`X.Y.Z` yes, `X.Y` or `latest` no) | public exact tag | **"Re-run all jobs"**, not "Re-run failed jobs": once `X.Y.Z` is tagged the current highest stable tag no longer equals the replayed plan's `highest_stable`, so the stale-plan guard would (correctly) fail a replay. A fresh run sees the tag, resumes with its digest, and completes |
| `github-release` | tags public, no release page | re-run |
| `quickstart` | release public, run red | fix the document or checker, re-run (idempotent) |
| run cancelled by a newer queued tag (OPS-27) | whatever the cancelled run had completed | re-run the cancelled run; confirm with `imagetools inspect` |
| a bad release reached `promote` | immutable public tag | publish a fixed patch version. Manual exception for `latest` only: a maintainer with a package-write token runs `docker buildx imagetools create -t ghcr.io/gizawnahom/ledgerops:latest ghcr.io/gizawnahom/ledgerops@<previous digest>` and edits the Release page to say the version is withdrawn. Signatures stay valid (they are per digest) |

### Concurrency (M2, OPS-27)

`concurrency: {group: release, cancel-in-progress: false}` queues runs, but GitHub keeps at most one running and one pending run per group: when a third run queues, the older **pending** run is cancelled. Pushing two release tags within roughly one release duration (about 20 minutes) can therefore cancel the first of them before it starts. `classify` prints a notice naming any other queued or in-progress release runs. After a cancelled release run: re-run it from the Actions page (a cancelled run can be re-run), or if the tag was never released, delete the tag and re-push; then confirm `docker buildx imagetools inspect ghcr.io/gizawnahom/ledgerops:X.Y.Z`. A per-tag group was rejected: two promotions could then run at once and `latest` could move out of order.

### Pre-tag digest-preservation check (m8)

Before creating any tag, `promote` runs `docker buildx imagetools create --dry-run -t <repo>:<version> <repo>@<digest>`, strips the trailing newline and takes the sha256 of the output; it must equal `<digest>`, otherwise the job fails and no tag exists. Verified locally 2026-10-17 on a multi-platform index (`alpine@sha256:294b683c…`): the dry-run output with its trailing newline removed hashes to the source digest, and equals `docker buildx imagetools inspect --raw` for it. This proves single-source `imagetools create` copies the index bytes unchanged, so the tag, the signature and the smoke all refer to the same digest. (Dry-run needs no registry write.)

### Item 5: cosign storage on GHCR (referrers vs tag fallback)

Evidence today: an anonymous `GET /v2/sigstore/cosign/cosign/referrers/<digest>` for a real public digest returned 404 `MANIFEST_UNKNOWN` on 2026-10-17, so GHCR is expected to use the tag fallback (`sha256-<hex>.sig`, `.att`). This is a probe of one repository, not proof about `gizawnahom/ledgerops`; the pipeline settles it on the first real run.

The `sign` job runs a step after signing and verification that records `signature_storage=tag-fallback|referrers|both` for the index digest (probe 1: `docker buildx imagetools inspect <repo>:sha256-<hex>.sig`; probe 2: `GET /v2/<repo>/referrers/<digest>` returns 200 with a non-empty `manifests` list) and fails only on `none`. The value is written to the run summary and KPI-R5. What protects consumers is outcome-based: the `quickstart` job runs `cosign verify` and `verify-attestation` **anonymously** after release, which holds in either storage mode. Consequences handled elsewhere: the classifier ignores `sha256-*` tags; orphan cleanup pairs a `.sig`/`.att` version with its subject digest; verification uses digest references throughout. DELIVER records the observed value in `kpi-contracts.yaml` after the first rc.

### Item 6: GitHub Release notes mechanism

`gh release create --help` (v2.102.0, downloaded and checksum-verified) documents: "Additional release notes can be prepended to automatically generated notes by using the `--notes` flag", and lists `-F/--notes-file`. The help does not say `-F` prepends. The source (`pkg/cmd/release/create/create.go`) shows `-F` reads the file into the same `Body` that `--notes` sets, and with `--generate-notes` and no `--notes-start-tag` the call sends `body` plus `generate_release_notes: true` to the API, which prepends the body. So the combination works; confirm on the first rc that the rendered page shows the template above the generated notes.

Mechanism: `envsubst` renders `notes.md` from `.github/release-notes.md.tmpl` with the image reference `ghcr.io/gizawnahom/ledgerops:X.Y.Z`, the index digest, the two-step SBOM lookup, `cosign verify` with the exact-case regex, and a pin-by-digest line (a golden-file test renders it with fixture values). Then `gh release create "$TAG" --verify-tag --title "$TAG" --notes-file notes.md --generate-notes` plus `--prerelease` when the plan says so and `--latest=<move_latest>`, all values taken from `env:` (never inline expressions). `--verify-tag` stops `gh` creating a tag from the default branch. Explicit `--latest` matters: without it GitHub marks the newest-by-date release as Latest, so a backport would take the badge. If `gh release view "$TAG"` succeeds the job skips creation (re-run); either way it then asserts the body contains the image reference.

### Item 7: orphaned untagged digests (manual, no automation in v0.1.0)

Orphans come from failed or superseded runs: an untagged index plus its untagged per-platform manifests, and signature artifacts. GHCR also lists the **per-platform manifests of every released index as separate untagged versions**, so "delete untagged" destroys releases. Cleanup is optional hygiene (public package storage is believed free; verify against GitHub's current packages billing page), so do it rarely.

1. Pre-checks: no `release.yml` run queued or in progress (`gh run list --workflow release.yml --status queued` and `--status in_progress`); a token with `read:packages,delete:packages` (`gh auth refresh -s read:packages,delete:packages`); work from a saved snapshot: `gh api --paginate /users/gizawNahom/packages/container/ledgerops/versions > versions.json`.
2. **Reachable set (B1)** is the union of: (a) every version carrying any of the tags `X.Y.Z`, `X.Y.Z-<prerelease>`, `X.Y` or `latest`; (b) the `sha256-<hex>.sig` and `sha256-<hex>.att` tagged versions for every digest in (a) and (c); (c) every child digest of each index in (a), found with `docker buildx imagetools inspect --raw ghcr.io/gizawnahom/ledgerops@<digest>`; (d) **every digest still referenced by a re-runnable run** (below).
3. **Re-runnable window (m7).** GitHub allows re-running a workflow run for a limited period (documented as 30 days; confirm on the Actions docs page before the first cleanup). The cleanup age floor is therefore **35 days**, and set (d) is collected from release runs younger than 35 days: `gh run list --workflow release.yml --created ">=<date>" --json databaseId`, then `gh run view <id> --log | grep -o 'image_digest=sha256:[0-9a-f]*'`. Their digests, children and signature tags are reachable even if untagged. **The cleanup aborts without deleting anything if any run in the window yields no `image_digest=` line** (for example expired logs); resolve that run first.
4. Candidates: versions not in the reachable set and created more than 35 days ago. Print id, name, tags, created_at and stop (dry run).
5. After human review, delete one by one: `gh api -X DELETE /users/gizawNahom/packages/container/ledgerops/versions/<id>`. A `sha256-<orphan>.sig/.att` version goes only together with its orphaned subject.
6. Verify: `docker buildx imagetools inspect` and an anonymous `cosign verify` on the latest stable tag and one older tag still succeed.

Unverified against live GHCR (no package exists yet); the API field names used in step 1 (`metadata.container.tags`, `name` = digest) are GitHub's documented shape and should be eyeballed on first use. If this is ever automated, use a multi-arch-aware tool (DDD-45).

### Item 8: first release and OQ-7 (owner runbook)

Required, in this order, by the owner:
1. First image version is `v0.2.0` (user-decided); the existing `v0.1.0` stays as is. Use `v0.2.0-rc.1` for the first run.
2. **Required before any rc tag (m10): create a repository ruleset on tags `v*` blocking update and deletion.** Verify with `gh api repos/gizawNahom/ledgerops/rulesets --jq '.[].name'` (the list is empty today). Optional: enable immutable releases (a published release then cannot be corrected, only superseded).
3. Push `v0.2.0-rc.1`. Expect `anon-probe` to fail with "package is not public: set it to Public, then re-run failed jobs". The digest is built and smoked (and signed once slice 07 exists).
4. GitHub profile → Packages → `ledgerops` → Package settings → Danger Zone → Change visibility → Public. There is no API for this and `GITHUB_TOKEN` cannot do it. Same page, "Manage Actions access": the repository `gizawNahom/ledgerops` must be listed with Write (the `org.opencontainers.image.source` label links it on first push; add it by hand if missing).
5. Actions → the failed run → **Re-run failed jobs** (per § Re-run semantics: reuses the earlier digest if GitHub behaves as documented, otherwise use "Re-run all jobs" and accept an orphan). Read `image_digest=… source=…` in the summary. Confirm `anon-probe` passes, then `docker logout ghcr.io && docker pull ghcr.io/gizawnahom/ledgerops:0.2.0-rc.1` from another machine.
6. Hand the first run's `signature_storage` and printed certificate identity to DELIVER for `kpi-contracts.yaml` (OPS-19, OPS-24).

---

## Wave: DEVOPS / [REF] Mutation testing strategy

**nightly-delta**, already persisted in `CLAUDE.md` (not asked, not rewritten). NOT run during feature delivery. One scope tweak (OPS-26): the `mutation-delta` file filter in `nightly.yml` widens to include `internal/release/`. Not added: `PlanMigration`, the SCRAM builder and config parsers (they sit in packages that also hold Testcontainers tests or in `package main`); if mutation coverage of them is wanted, DELIVER should put the pure parts in a package without database tests.

## Wave: DEVOPS / [REF] Observability stack

Unchanged (OPS-5): logs via `log/slog` JSON to stdout, metrics at `GET /metrics` (Prometheus text), no traces, Prometheus/Grafana scaffold in the demo compose only. Release-side signals are the job summaries above. One optional item for DELIVER, not a requirement: a `HEALTHCHECK` in the Dockerfile needs no `RUN` (alpine's busybox `wget` exists) and would let client compose files use `service_healthy`; Kubernetes users would use their own probes against `/health`.

## Wave: DEVOPS / [REF] Branching strategy

Trunk-based (OPS-8), unchanged for development. Releases: an annotated or lightweight tag `vMAJOR.MINOR.PATCH[-prerelease]` on a commit of `main` is the only publish trigger (D2). Tags are not CI triggers any more (`ci.yml` push is branches-only); `release.yml` calls `ci.yml`. Branch protection on `main` keeps its required jobs and gains `workflow-guards`. Backports use a short-lived `release/X.Y` branch. Recommended owner setting: a ruleset on tags `v*` (OPS-22).

## Wave: DEVOPS / [REF] Coexistence matrix

| Tool or path | Must not break | Note |
|---|---|---|
| `make up`, `demo-01..04`, `chaos-01`, `race-02`, `race-03` | yes | now build the console (slower) and migrate through the shipped `ledgerops migrate`; compose sets `LEDGEROPS_APP_PASSWORD=app-secret` on `migrate` |
| `docker-compose.yml`, `docker-compose.loadtest.yml`, `load-test.yml` | yes | service names `postgres`, `migrate`, `app` unchanged |
| golang-migrate (library, embedded) | yes | the `migrate/migrate` container image is retired |
| `ci.yml` required jobs on `main` | yes | names unchanged; one new required job |
| `nightly.yml` (`mutation-delta`, `slice-cycle-time`, `demo-cold`) | yes | gains pins and the scope tweak only. `demo-cold` pays the same Node-stage cost |
| ledger-core-console (`web/console`, Pact, Vite) | yes | Node 20 to 22 in CI; Pact provider test keeps running |
| Prometheus/Grafana scaffold | yes | untouched |
| existing GitHub pre-release `v0.1.0` and tag `pre-multitenancy` | yes | not modified (OPS-25, user-decided) |
| pre-commit | not installed | inherited; no local hook added |

## Wave: DEVOPS / [REF] Pre-requisites

DESIGN constraints the platform satisfies:
1. Digest-first order, tags promoted last, `latest` last and only for the highest stable (ADR-020): jobs 3-9 above.
2. Frozen `release.yml` identity, signing in-file: stated at the top of the outline.
3. Least-privilege per-job permissions, global non-cancelling concurrency (ADR-020 items 5-6): in the outline; the pending-run cancellation caveat is OPS-27.
4. Every `uses:` by SHA, every `FROM` by digest, tool binaries by sha256, automatically checked where checkable (DDD-44): § Supply-chain pins.
5. Both architectures smoked natively, nothing built under emulation (ADR-020 item 9, DDD-34): OPS-15 and the image topology.
6. Cosign 2.x, digest references only, no reliance on referrers (ADR-021 item 3): OPS-19.
7. Dedicated clusters for role-mutating tests (Test harness impact): `container-acceptance` (OPS-28) and the fixture environments.

Needed before DELIVER slice 03: lowercase the image name in the workflow (`ghcr.io/gizawnahom/ledgerops`; GHCR requires lowercase and `github.repository` is `gizawNahom/ledgerops`); the `v*` tag ruleset (OPS-22) before the first rc.

## Wave: DEVOPS / [REF] Changed Assumptions

Details and quotes are in `devops/upstream-changes.md`. Items marked "user-decided" were settled 2026-10-17.

1. **Node stage**: DESIGN "Node 20 stage, Vite 5 (existing)" → `node:22-slim`, glibc (user-decided).
2. **Runtime base**: DESIGN "alpine:3.20 (existing)" → `alpine:3.24` (user-decided).
3. **Build topology**: ADR-020 step 3 "build per platform (amd64, arm64), push by digest ..., merge into an untagged index" → one multi-platform build, no merge job; provenance and BuildKit SBOM off.
4. **Signing identity**: ADR-021 item 5 lowercase `github.com/gizawnahom/…` → exact case `gizawNahom` (user-decided).
5. **First version**: D14 "First version is `v0.1.0`" → `v0.2.0`; the existing `v0.1.0` is untouched (user-decided).
6. **SBOM tooling**: syft via a pinned tarball, no `sbom-action`.
7. **Tag namespace**: tag-fallback signature tags appear in the package; classifier and cleanup handle them.
8. **CI coverage**: three packages have no CI job today; OPS-23 and OPS-28 add them.
9. **Resume and re-run wording** (ADR-020 items 4, 11) and **"re-evaluate `moveLatest` at that moment"** → stale-plan guard (OPS-29, OPS-30).
10. **`ci` gating on the resume path**: ADR-020 order lists `ci` once; every downstream job now needs it (OPS-14).

## Wave: DEVOPS / [REF] Open questions

None outstanding for the user: the four DEVOPS questions were decided on 2026-10-17 (OPS-17, OPS-24, OPS-25).

For DISTILL:
1. The assertion fields of each `KPI-R*` entry are the observable outputs the harness must print.
2. Fixtures are defined in `environments.yaml`, including the frozen pre-feature migrations (`internal/adapters/postgres/prefeature/`).
3. A `workflow-guards` scenario for K2: every job after `classify` needs `ci`; `promote` needs `smoke` and `anon-probe` (and `sign` from slice 07); Dockerfile topology (M3).
4. The identity-regex match runs against the real certificate on the first rc, not a mock.
5. Classifier scenarios: signature-tag noise (`sha256-<hex>.sig`) in the listing, and a 404 tag list for an unknown package (first run).
6. `serve` must not receive `LEDGEROPS_MIGRATE_DSN` in the demo compose (shared `env_file` regression).
7. Pre-tag digest-preservation check and stale-plan guard are testable shell; give them scenarios.

For DELIVER (optional): pin `postgres:16` in the six test files; if mutation coverage of `PlanMigration` or the SCRAM builder is wanted, put the pure parts in a package without database tests.

## Wave: DEVOPS / [REF] Wave Decisions Summary

# DEVOPS Decisions — container-image-release

## Key Decisions
- [OPS-14] One multi-platform build pushed by digest; digest-first order; `ci` gates every job including the resume path; provenance off
- [OPS-15] arm64 smoke native on `ubuntu-24.04-arm`, staged into slice 06; nothing built under emulation
- [OPS-16] Pin everything with real values and verified checksums; zizmor hash-pin audit; Dependabot
- [OPS-17] Node 22 slim and `alpine:3.24` (user-decided)
- [OPS-18] KPI-R1..R9 wired; KPI-5 re-baselined (71/70/68 s now, about 135 s expected, 300 s kept, guardrail emitter at 200 s)
- [OPS-19..22] Signature-storage check, `gh release create` mechanism, manual orphan cleanup (35-day floor), owner runbook with the tag ruleset required
- [OPS-23] `ci.yml` changes; compose `migrate` uses the shipped image
- [OPS-24, OPS-25] Exact-case identity regex; first release `v0.2.0` (both user-decided)
- [OPS-26] Mutation scope adds `internal/release/`
- [OPS-27] Global concurrency group kept; pending-run cancellation documented, notice emitted
- [OPS-28] New required job `container-acceptance`
- [OPS-29, OPS-30] `promote` runs no repo code; stale-plan guard; re-run semantics reconciled with the ADR

## Infrastructure Summary
- Deployment: none hosted; clients self-host the signed GHCR image; Recreate, rollback by previous digest
- CI/CD: GitHub Actions; `ci.yml` (branches, PRs, called) plus `release.yml` on strict version tags; trunk-based
- Observability: slog JSON, `/metrics`, job summaries for release KPIs
- Mutation testing: nightly-delta

## Constraints Established
- `release.yml` path frozen; only tag runs sign; all `uses:` and `FROM` pinned and checked
- Console build and CI use the same Node major
- `app` never receives `LEDGEROPS_MIGRATE_DSN`, including via shared env files
- Tag-fallback signature tags are not versions
- No `RUN` after the final `FROM`

## Upstream Changes
- See `devops/upstream-changes.md`
