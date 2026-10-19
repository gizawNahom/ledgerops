# Slice 06 — Get the right tags, architectures and release page

**Story**: US-6 | **job_id**: J11 | **persona**: P1 | **Priority**: 6

## Goal
A stable tag publishes `X.Y.Z`, `X.Y` and `latest` for both linux/amd64 and
linux/arm64 with OCI labels and a GitHub Release; prereleases publish only
their exact version.

## IN scope
- Tag set: exact, minor, `latest` (stable only); prereleases exact only
- `latest` moves only for the highest stable version, and only after the smoke check passes
- Multi-arch index (amd64, arm64)
- OCI labels: version, revision, source
- GitHub Release per tag; prerelease flagged correctly; names the image reference
- Older exact tags never move

## OUT scope
- Signing and SBOM (slice 07)
- Changelog tooling beyond GitHub's generated notes (OQ-12)
- Other registries or architectures

## Learning hypothesis
Disproves: "tag classification can be decided from the tag string and the
existing tag list alone." If a backport (`v0.2.2` after `0.3.0`) moves `latest`,
or an rc moves `0.3`, the rules in the workflow are wrong before a real user
is affected.

## Acceptance criteria
- [ ] `v0.2.0`: `0.2.0`, `0.2`, `latest` on one digest
- [ ] `v0.3.0-rc.1`: exact tag only; GitHub Release marked pre-release
- [ ] `v0.2.1` moves `0.2`; `0.2.0` unchanged
- [ ] `v0.2.2` after `0.3.0` moves `0.2` but not `latest`
- [ ] Index lists both architectures
- [ ] Labels correct
- [ ] GitHub Release exists for each tag; body names image reference, digest and verify command, followed by auto-generated notes

## Dependencies
Slice 03 (pipeline), slice 05 (version stamped into the binary and label).

## Production data
Publish real tags `v0.2.0-rc.2` (if needed), `v0.2.0`, `v0.2.1` on the real repository (the source-only `v0.1.0` is untouched);
inspect the real registry. Pull on an Apple Silicon machine for the arm64
check.

## Effort estimate
At most 1 day. Reference class: docker metadata and buildx configuration.

## Taste-test note
Four items (tags, arm64, labels, Release), all configuration of one workflow.
If it exceeds one day, split arm64 into slice 06b rather than stretching.

## Pre-slice SPIKE
Not required. Arm64 smoke under emulation is an OQ-6 question for DESIGN.
