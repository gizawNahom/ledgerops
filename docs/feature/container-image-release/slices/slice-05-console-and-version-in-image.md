# Slice 05 — Open the console and see the version from the image

**Story**: US-5 | **job_id**: J11 | **persona**: P2 (also serves P1) | **Priority**: 5

## Goal
A container started from the image serves the operator console at `/console`
and reports its own version and commit.

## IN scope
- SPA bundle built during the image build and served by the image with nothing mounted from outside
- A failed console build fails the image build (no silent 404 image)
- `version` command printing version and commit; `dev` when none supplied
- Version stamped from a build argument into the binary
- Smoke check extended to assert `/console` returns 200 and `version` matches the tag

## OUT scope
- Console features or API changes
- OCI labels (slice 06)
- A version field in HTTP responses

## Learning hypothesis
Disproves: "copying the binary is enough to ship the console." The current
convention (`web/console/dist` relative to the working directory, gitignored,
silently skipped when absent, `console_static.go:31,45-49`) predicts the first
pulled image will 404 on `/console`; if the build cannot place the bundle
without a code change, OQ-5 needs a DESIGN answer.

## Acceptance criteria
- [ ] `/console` returns 200 and the SPA shell from a freshly built image with no mounts
- [ ] `/console/verdict` still requires the operator key
- [ ] `version` prints version and commit; unreleased build prints `dev`
- [ ] Image build fails when the console build fails
- [ ] Smoke check asserts `/console` 200

## Dependencies
None to start; smoke assertion needs slice 03.

## Production data
Run the real image against the real compose Postgres with the real SPA bundle;
open the console in a browser and read the verdict after `make demo-01`.

## Effort estimate
At most 1 day. Reference class: adding a Node build stage to an existing
two-stage Dockerfile plus a version flag.

## Pre-slice SPIKE
Not required.
