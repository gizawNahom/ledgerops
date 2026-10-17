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

Scenario: Migration refuses to run without a password
Given Priya sets neither the password nor the password file
When she runs `migrate` against the empty database
Then the command exits non-zero naming `LEDGEROPS_APP_PASSWORD` and `LEDGEROPS_APP_PASSWORD_FILE`
And no table exists in the database afterwards
And `ledgerops_app` does not accept `app-secret`

Scenario: No moment exists when the app role accepts a password the operator did not supply
Given a migration run that stops after the first schema change
When someone tries to log in as `ledgerops_app` with `app-secret`
Then the login is refused

Scenario: A database with any name can be prepared
Given the database is named `kestrel_ledger`, not `ledgerops`
When Priya runs `migrate`
Then it succeeds and `ledgerops_app` can connect to `kestrel_ledger`

Scenario: A mismatched password is caught at app start, not at the first request
Given the database was migrated with `T7!kq@9x%Zm`
When Priya starts the app with an app DSN carrying a different password
Then the app exits non-zero with a startup refusal naming the role and that authentication failed
And the log line does not contain either password

Acceptance Criteria
- [ ] `migrate` applies the full migration set to an empty database and sets the app role password from `LEDGEROPS_APP_PASSWORD` or its `_FILE` variant; a trailing newline in the file is not part of the password
- [ ] `migrate` without a usable password (unset, empty) exits non-zero before changing the database and names both variables
- [ ] At no point, including after an interrupted run, does `ledgerops_app` authenticate with a password the operator did not supply
- [ ] Migration works for a database with any name
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
- The privileged role needs enough privilege to create and alter roles (OQ-8).
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

Scenario: Key values are never printed
Given Tomasz sets `LEDGEROPS_OPERATOR_KEY` to `op-9f3b71c2`
When the container starts, successfully or not
Then `op-9f3b71c2` appears in no log line

Acceptance Criteria
- [ ] Unset or blank `LEDGEROPS_OPERATOR_KEY` or `LEDGEROPS_APP_DSN` causes exit 1 before any database connection (D9 as revised)
- [ ] Unset or blank `LEDGEROPS_DEMO_TENANT_KEY` is accepted and turns the demo-tenant mapping off; no token resolves to `tnt_legacy_seed` through it
- [ ] One message lists every missing required variable
- [ ] Key and DSN values never appear in logs
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

Scenario: A published image is proven usable
Given `0.1.0` is published
When the pipeline's final check pulls that image, migrates a fresh Postgres and starts it
Then `/health` answers 200

Scenario: A version that fails its usability check is not presented as a release
Given the published image cannot migrate a fresh database
When the final check fails
Then the run ends red naming the failing step and the version is not announced as usable

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
- [ ] After publishing, a check pulls the published image, migrates a fresh Postgres with a generated password, starts it, and requires `/health` 200
- [ ] A failed check leaves the run red and the version not announced as usable
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
And the message says whether schema changes were already applied

Acceptance Criteria
- [ ] An existing database created by the demo stack at an earlier schema version is migrated to head with no entry, balance or trial-balance change
- [ ] An explicit supplied password is always applied, including when `ledgerops_app` already exists
- [ ] Re-running with the same inputs is a successful no-op
- [ ] The append-only guarantee (UPDATE and DELETE refused for `ledgerops_app`) holds after upgrade
- [ ] Insufficient privilege fails with a message naming it and does not change the password
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
Then both appear, and the rc is marked pre-release, each naming the image reference

Acceptance Criteria
- [ ] Stable tags publish `X.Y.Z`, `X.Y`, `latest`; prereleases publish the exact version only
- [ ] `latest` moves only when the new stable version is the highest stable version
- [ ] Older exact tags are never moved
- [ ] The image index contains linux/amd64 and linux/arm64
- [ ] OCI labels for version, revision and source are present and correct
- [ ] A GitHub Release is created for each tag, prerelease flagged correctly
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

Scenario: The software bill of materials can be retrieved
Given release `0.2.0`
When Tomasz fetches its SBOM attestation
Then it lists the Go modules and base-image packages in the image

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
- [ ] An SBOM attestation is published for every release
- [ ] A signing or attestation failure fails the release and `latest` does not move
- [ ] Both architectures are covered (OQ-10 settles how)

Outcome KPIs
- Who: operators about to deploy the image
- Does what: verify provenance before running
- By how much: 100% of stable releases verify and carry an SBOM
- Measured by: final release step runs `cosign verify` and `verify-attestation`
- Baseline: none

Technical Notes
- SBOM format and what is signed (index vs per-platform) are OQ-10.
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
Then it tells him to run `migrate` with his own password and restart the app

Scenario: The verification command is documented with the real identity
Given the quickstart
When Priya copies its verify command
Then it succeeds against a release

Acceptance Criteria
- [ ] The quickstart's commands run verbatim against the published image in CI
- [ ] README Quickstart leads with the published image; the limitation line is removed or reworded; roadmap item "Adoptable" is amended to say what remains (OpenAPI, transfer metadata)
- [ ] No demo credential is used in the published-image path
- [ ] Upgrade, rotation and verification sections exist
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
6. A real rc tag (`v0.1.0-rc.1`) has been published, smoke-passed and, once slice 07 lands, verified
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

DoR status: PASSED for all eight stories. Re-run 2026-10-17 after the amendments: still PASSED. US-2 now has 8 scenarios (within the 3 to 7 guideline by one; the three demo-key scenarios are short variants of one rule, so the story is not split, noted for DESIGN). Carried: P1 archetype caveat (inherited), OQ-7 owner action. OQ-1, OQ-11, OQ-12 closed. Peer review (`nw-product-owner-reviewer`) was not run in this dispatch; it is optional per the DISCUSS skill and the mandatory consolidated review fires at the end of DISTILL.

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
    And migrate is killed after the first schema change
    When someone logs in as "ledgerops_app" with "app-secret"
    Then the login is refused
    And re-running migrate with "T7!kq@9x%Zm" completes and that password works

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
    And the output says whether schema changes were applied

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
    And the smoke check migrated a fresh database and saw /health 200

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
