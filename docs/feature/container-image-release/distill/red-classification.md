# RED classification — container-image-release (pre-DELIVER gate)

Run 2026-10-20 on branch `feature/container-image-release`. Method: the default run (WS only), then every `@pending` scenario selected by `LEDGEROPS_AT_TAGS` in batches (`go test -count=1 -v ./tests/acceptance/containerimagerelease/`). Totals (revision cycle 1): 131 scenarios after outline expansion; 1 walking skeleton enabled, 130 `@pending`.

Legend: MF = MISSING_FUNCTIONALITY (correct RED: the assertion or precondition fails because the feature is absent). GUARD = green by design from day one (regression guard on a fixture). SKIP = `@requires_external`, skips itself unless `LEDGEROPS_EXTERNAL` is set. No scenario was classified IMPORT_ERROR, FIXTURE_BROKEN, SETUP_FAILURE, WRONG_ASSERTION or OBSERVABLE_NOT_AT_PORT; no scenario passed vacuously after the two fixes below.

| Feature file | Scenarios | Result | Failing reason (shared) |
|---|---|---|---|
| walking-skeleton | 1 (enabled) | MF | Given "the database has been prepared with migrate" fails: the current binary ignores `migrate`, serves and exits 1 ("precondition not met ... expected migrate to succeed, exit 1") |
| milestone-01 (US-1a) | 26 | 26 MF | binary has no `migrate`; logins as `ledgerops_app` refused because the password was never set; refusal tokens (`LEDGEROPS_APP_PASSWORD`, `only one`, `dirty`, `CREATEROLE`...) absent from output. The four interrupted-run scenarios (killed between files, killed in a file, and their re-run halves) drive the real `migrate` binary; today it never reaches the hold point, reported as `migrate never reached the hold point` (MF, a fixture-dependency on `migrate` existing, not a harness error) |
| milestone-02 (US-1b) | 9 | 9 MF | preconditions need a working `migrate` (named in the failure text) or the cluster-rule message is absent |
| milestone-03 (US-2) | 18 | 18 MF (incl. 2 image rows) | refusal output does not name the variables (current binary dies on a DSN parse / connection error); `GET /health` is the 501 scaffold; unknown subcommand not handled; demo compose still uses `migrate/migrate` |
| milestone-04 (US-4) | 17 | 16 MF, 1 GUARD | upgrade not performed; password not rotated; privilege refusals not produced. GUARD: "The frozen demo database really does accept the demo password" (fixture sanity, passes today by design) |
| milestone-05 (US-5) | 9 | 9 MF | `version` not a command; `go build -tags console_embed` succeeds without a bundle (no embed file yet); Dockerfile has no console stage |
| milestone-06 (US-3/US-6 classifier examples) | 10 | 10 MF | `scripts/releaseplan` is a RED scaffold (exit 70, `__SCAFFOLD__`); the 5 properties are now `internal/release` unit tests (RED with `LEDGEROPS_RUN_PENDING=1`, otherwise skipped) |
| milestone-07 (release pipeline) | 30 | 27 MF, 3 SKIP | `.github/workflows/release.yml`, `.github/release-notes.md.tmpl`, `examples/published-image/` do not exist; Dockerfile `FROM` lines carry no digest; `scripts/smoke` is a RED scaffold (exit 70) |
| milestone-08 (US-7) | 5 | 1 MF, 4 SKIP | identity pattern cannot be read (no `release.yml`); four real-registry scenarios skip |
| milestone-09 (US-8) | 7 | 6 MF, 1 SKIP (deferred quickstart run) | `docs/quickstart-published-image.md` does not exist; README still leads with build-from-source |

Summary (approximate by file above): the enabled walking skeleton and every enabled-by-tag scenario classify MF; 1 GUARD (frozen-fixture sanity, now step `the app role logs in with "app-secret"`); 8 SKIP (`@requires_external`, including the deferred quickstart run). New in this cycle: the resumed-tag commit check (MF: `scripts/releaseplan` scaffold output does not name the commits). Every failure message names the missing capability; none is a harness error.

Two scenario defects found by this gate and fixed before handoff (they passed vacuously in RED):
1. milestone-01 "Migrate output never contains the password": three rows asserted only absence of the secret. Now each row also requires a refusal naming a situation-specific token.
2. milestone-03 "Key values are never printed": the `set` row asserted only absence. Now it uses refusing faults and requires the refusal to name the variable.

Notes for DELIVER:
- Order of first enablement follows the slices: WS, then milestone-01, 03 (config refusals), 07 structural + promote, ... Image scenarios (`@image`) need a Docker daemon and run only in `container-acceptance`.
- `@image` serve-refusal rows classify MF with the image built from the current Dockerfile (entrypoint `ledgerops-api`).
- Wall-clock: milestone-01 batch about 55 s, milestone-04 about 45 s, the `@image` batch about 30 s plus one image build. Each `@dedicated-cluster` scenario starts its own `postgres:16`.
- Guard scenario to keep green forever: the frozen-fixture sanity row; the Go guard tests in `internal/adapters/postgres/prefeature` (one skipped until slice 01a pins the edited 0001).
