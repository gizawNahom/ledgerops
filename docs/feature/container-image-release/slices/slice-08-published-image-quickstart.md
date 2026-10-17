# Slice 08 — Follow a quickstart that uses the published image

**Story**: US-8 | **job_id**: J11 | **persona**: P1 | **Priority**: 8

## Goal
A developer with only Docker can follow written steps from `docker pull` to a
first posted transfer, and an operator can follow the upgrade, rotation and
verification steps.

## IN scope
- A published-image quickstart (docker run or compose snippet, separate from `docker-compose.yml`)
- Generated password and keys, never demo values; warning against `app-secret` and `demo-*` keys
- Upgrade and rotation section (restart the app after rotation)
- Verification section with the real cosign identity
- README Quickstart leads with the published image; "no published container image; you build from source" limitation removed; roadmap "Adoptable" amended to what remains
- CI runs the quickstart's commands verbatim against the published image

## OUT scope
- OpenAPI spec, SDKs, transfer metadata
- GitHub Pages site (waits on the Adoptable milestone, a separate decision)
- Kubernetes or Helm guidance

## Learning hypothesis
Disproves: "the image is self-explanatory once published." If a reader of the
docs alone cannot reach a transfer, or the CI run of the documented commands
fails, the migrate-then-start order or the secret handling is not obvious
enough and needs a product change, not only more prose.

## Acceptance criteria
- [ ] Documented commands pass verbatim in CI against the latest release
- [ ] README and limitations lines updated
- [ ] No demo credentials in the published-image path
- [ ] Upgrade, rotation and verification sections present
- [ ] Example compose or `docker run` file present and separate from the demo compose

## Dependencies
Slices 03 to 07 (real tag, real signing identity).

## Production data
Run the quickstart on a machine with no checkout against the real `v0.1.0`
image the same day it is written.

## Effort estimate
At most 1 day. Reference class: README and docs edits plus one CI job.

## Pre-slice SPIKE
Not required.
