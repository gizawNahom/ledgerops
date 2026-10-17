# Slice 03 — Pull a pinned release from GHCR

**Story**: US-3 | **job_id**: J11 | **persona**: P1 | **Priority**: 3 (walking skeleton)

## Goal
Pushing a version tag on a green commit publishes an amd64 image to GHCR whose
exact-version tag has been proven to migrate a fresh Postgres and serve
`/health`.

## IN scope
- Trigger on `v*.*.*` tags (prerelease suffix allowed)
- Reuse the full `ci.yml` suite as a gate (`workflow_call`)
- Build and push amd64, exact version tag only (version without `v`)
- Smoke check: pull the image by digest before any version tag exists (each published architecture; amd64 here), migrate a fresh Postgres with a generated password, start with generated keys, require `/health` 200; the version tag appears only after smoke passes
- Failed smoke leaves the run red and no version tag exists for it
- Non-matching tags and branch pushes publish nothing; existing versions never overwritten

## OUT scope
- `X.Y`, `latest`, arm64, labels, GitHub Release (slice 06)
- Signing and SBOM (slice 07)
- Console and version assertions in the smoke check (slice 05 adds them)

## Learning hypothesis
Disproves: "the compose-based `ci.yml` can be reused as a release gate and a
tag-built image behaves like the locally built one." If `workflow_call` reuse
or the duplicate tag-triggered run (OQ-6) forces a rewrite of `ci.yml`, that is
learned here, on an rc tag, not at the first real release.

Confirms if it succeeds: a client can `docker pull` a pinned version that was
verified against a real database.

## Acceptance criteria
- [ ] `v0.1.0-rc.1` publishes `ghcr.io/gizawnahom/ledgerops:0.1.0-rc.1`
- [ ] A failing CI job on the tagged commit publishes nothing
- [ ] Smoke check passes against the pushed digest (before tagging) and fails visibly, with no version tag created, when the image cannot migrate
- [ ] `v1.2`, `nightly-*` tags and branch pushes publish nothing
- [ ] Re-pushing an existing version tag is refused; digest unchanged
- [ ] Anonymous pullability is checked on the pushed digest before any version tag is promoted; the first run is expected to fail this step until the owner makes the package public (OQ-7), then a re-run passes

## Dependencies
Slices 01a and 02. Owner action: make the GHCR package public (OQ-7).

## Production data
Real tag push on the real repository (`v0.1.0-rc.1`), real GHCR, real Postgres
service container; dogfood by pulling the rc on Gizaw's machine the same day.

## Effort estimate
At most 1 day. Reference class: a standard docker publish workflow plus one
`workflow_call` conversion. Risk is the `ci.yml` reuse.

## Pre-slice SPIKE
Recommended, 1 hour: run `ci.yml` as a called workflow from a throwaway tag
workflow on a branch to see whether the compose-based jobs survive reuse and
what the double run looks like.
