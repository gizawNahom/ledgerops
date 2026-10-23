# Slice 07 — Verify the image came from the project's pipeline

**Story**: US-7 | **job_id**: J12 | **persona**: P2 | **Priority**: 7

## Goal
Every published tag carries a keyless cosign signature and an SBOM attestation
that an operator can verify with one command each.

## IN scope
- Keyless signing of each published tag by the release workflow
- SBOM attestation per release
- Final workflow step runs `cosign verify` and `cosign verify-attestation` and fails the release otherwise
- `latest` moves only after signing and attestation succeed
- Prereleases signed too

## OUT scope
- Admission policies, key management, transparency-log operations
- Vulnerability scanning gates
- Documenting the command (slice 08)

## Learning hypothesis
Disproves: "keyless signing from the release workflow can be verified by an
outsider with a stable, documentable identity." If the certificate identity
changes between a tag push and a reusable-workflow call (OQ-6), the command in
the docs would be wrong; this slice finds out on an rc.

## Acceptance criteria
- [ ] `cosign verify` with the repository identity succeeds for each published tag
- [ ] A look-alike image under the same name fails
- [ ] SBOM attestation is per platform digest: resolve the platform digest, fetch and verify that digest's attestation (not via the tag); index signature still verifies on the tag; lists Go modules and base packages for that platform
- [ ] Signing or attestation failure turns the release red and `latest` does not move
- [ ] Both architectures covered: the index and each platform manifest are signed, and each platform carries its own SBOM attestation

## Dependencies
Slices 03 and 06.

## Production data
Sign and verify the real `v0.2.0-rc.1` image in the real registry; try to
verify a locally pushed look-alike.

## Effort estimate
At most 1 day. Reference class: standard cosign and SBOM actions in a publish
workflow. Risk: identity string with workflow reuse.

## Pre-slice SPIKE
Optional, 1 hour: sign a throwaway image from a tag-triggered workflow and
print the certificate identity before writing any docs.
