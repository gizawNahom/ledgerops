# ADR-021: Keyless cosign over the index and every platform manifest; per-platform SPDX-JSON SBOM attestations verified by platform digest

**Status**: Accepted, 2026-10-17, revised in review cycle 1 (DDD-38, DDD-43)
**Owner**: nw-solution-architect (DESIGN, `container-image-release`)

## Context

D7 requires cosign keyless signing and an SBOM attestation (US-7, J12,
`cosign verify` and `verify-attestation`). A tag resolves to the index digest;
an attestation is attached to the digest it was made against. SBOM content
differs by platform (apk packages), so one index-level document cannot be
honest about both.

## Decision

1. **Sign by digest before any tag exists**: `cosign sign --recursive` over the
   index digest, which also signs each platform manifest. `cosign verify
   image:tag` therefore works on the tag, and a pull by platform digest is
   covered too.
2. **SBOM: one per platform, verified by platform digest.** syft (Apache-2.0)
   generates SPDX-JSON per platform digest; each is attached with
   `cosign attest --type spdxjson` to that platform's digest. The verify target
   for the SBOM is the platform digest, not the tag. Consumers resolve it in
   one step (`docker buildx imagetools inspect <tag>` or `crane digest
   --platform linux/amd64 <tag>`), then run `cosign verify-attestation --type
   spdxjson <repo>@<platform-digest>`. The quickstart and the GitHub Release
   body show the exact two commands. This avoids a misleading merged SBOM and
   avoids depending on a `--platform` flag in cosign.
3. **Version assumption**: cosign 2.x (2.2 or later, pinned by SHA and version
   in the workflow); verification uses digest references throughout, so no
   behaviour that differs between OCI referrers and the tag-fallback storage
   (`sha256-<digest>.sig/.att`) is relied on; GHCR works with the tag fallback.
4. **Verification in the pipeline**: after signing, the workflow runs `cosign
   verify` on the index digest and on each platform digest, and `cosign
   verify-attestation` on each platform digest, all before promotion. Failure
   fails the release and `latest` never moves. Prereleases are signed too.
5. **Published identity** (docs, quickstart):
   `^https://github.com/gizawnahom/ledgerops/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+(-.+)?$`
   with issuer `https://token.actions.githubusercontent.com`.
6. BuildKit SLSA provenance may be emitted for free but is not gated or
   promised.

## Alternatives considered

- **Also attest the index digest** (so `verify-attestation image:tag` works). The
  content would have to be one merged or arbitrarily chosen SBOM; a single
  document that is correct for only one platform is worse than a two-step
  lookup. Rejected.
- **`cosign verify-attestation --platform`.** Depends on a flag whose support
  across cosign minor versions is not verified here. Rejected as an assumption.
- **Sign the index only.** Per-platform digests become unverifiable.
- **CycloneDX.** Sound, but less alignment with GitHub dependency tooling and
  no gain for the stated verify flow.
- **BuildKit `--sbom`.** Unsigned in-toto attestations that
  `cosign verify-attestation` does not read.
- **GitHub-native `attest-*` actions.** Pins consumers to the `gh` CLI,
  contrary to the stories' `cosign verify`.

## Consequences

- US-7's scenario "The software bill of materials can be retrieved" and US-8's
  quickstart verify section change from tag-based to platform-digest-based SBOM
  retrieval (upstream-changes item 9).
- The certificate and repository name enter the public Rekor log (repo is
  public; acceptable).
- Depends on ADR-020's frozen `release.yml`.

## Amendments from DEVOPS (2026-10-19; `devops/upstream-changes.md` items 2, 5, 6, 7)

The text above is kept as decided; these amend it (DDD-38 updated in place).
- **Identity regex, exact case (item 2, user-decided 2026-10-17):** decision 5 becomes `^https://github.com/gizawNahom/ledgerops/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+(-.+)?$`, issuer `https://token.actions.githubusercontent.com`. The certificate carries the GitHub owner's real case; the image reference stays lowercase (`ghcr.io/gizawnahom/ledgerops`, GHCR requires it). The first rc's `sign` job prints the real certificate identity and asserts the published regex matches it (Earned Trust: probe the claim on the first real signature).
- **syft installation (item 6):** syft v1.54.1 is installed from its release tarball with a pinned sha256 (not through `anchore/sbom-action`). SPDX-JSON per platform digest and `cosign attest --type spdxjson` are unchanged.
- **Provenance (item 5):** decision 6 changes from "may be emitted" to "not emitted": BuildKit provenance and SBOM attestations are off so the index holds exactly the declared platforms.
- **Signature storage (item 7):** decision 3's tag fallback is confirmed by probe: signatures and attestations live as `sha256-<hex>.sig` / `.att` tags in the same package. Consumers and cleanup tooling must treat them as artifacts of the subject digest, not as versions (ADR-020 amendment).
