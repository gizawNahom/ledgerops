# ADR-020: Digest-first release pipeline; tags promoted last; `release.yml` is a frozen public identity

**Status**: Accepted, 2026-10-17, revised in review cycle 1 (DDD-35, DDD-36, DDD-37, DDD-44, DDD-45, DDD-46)
**Owner**: nw-solution-architect (DESIGN, `container-image-release`)

## Context

D1-D3 and D8 set GHCR, strict `vX.Y.Z` tags, a `workflow_call` reuse of
`ci.yml`, and a smoke test of the published image. A tag push also fires
`ci.yml`'s own `push:` trigger. `latest` must move last and only for the
highest stable version (a backport must not move it).

## Decision

1. **`release.yml`** triggers on version-tag push (glob), and re-validates
   strict semver in its first job. `ci.yml` gains `workflow_call` and its
   `push:` becomes branches-only (no duplicate run; arbitrary tag pushes no
   longer run CI, harmless because the commit was CI'd on its branch).
2. **Order**:
   1. `classify` (the pure `ReleasePlan`, below)
   2. `ci` (workflow_call)
   3. build per platform (amd64, arm64), push by digest with no tag, merge into
      an untagged index
   4. smoke by digest on both architectures, plus the upgrade leg
   5. sign the index recursively, generate and attest per-platform SBOMs, then
      verify signature and attestations (ADR-021)
   6. **anonymous-pull probe of the digest** (an unauthenticated manifest
      request by digest): a private package fails here, before any version tag
      exists
   7. guard that the exact tag is absent (or equals this digest), promote
      `X.Y.Z`
   8. promote `X.Y` and `latest` (stable and highest only)
   9. GitHub Release (prerelease flagged; body is a template that names the
      image reference, digest and the verify command, followed by
      auto-generated notes; the job asserts the body contains the image
      reference)
   10. quickstart check (ADR below, "Harnesses") against the just-released tag
   A failure in steps 3-6 leaves an untagged digest and no version tag.
3. **Pure classifier**: `internal/release` maps (tag, existing stable
   versions) to a `ReleasePlan` value (version, tags, prerelease, moveLatest);
   invoked by a thin `scripts/releaseplan/main.go`. The workflow only executes
   the plan. Table and property tests cover strict semver, prerelease, minor
   tag, backport, and re-run.
4. **Re-run semantics (DDD-45)**: builds are not claimed reproducible. If
   `X.Y.Z` already exists in GHCR, the run resumes: it reuses that tag's digest,
   skips build and push, re-runs smoke and verification against it, and
   completes any missing promotion (`X.Y`, `latest`, GitHub Release),
   re-evaluating `moveLatest` at that moment. If the tag is absent, the run
   builds anew; digests orphaned by earlier failed runs are ignored. No
   automatic pruning in v0.1.0: GHCR lists the untagged per-platform
   manifests of every index as separate untagged versions, so a naive "delete
   untagged" job would break released images. If pruning is added later it must
   use a multi-arch-aware tool. Orphaned digests are unsmoked or unsigned and
   pullable by digest only; documentation states that a release is a signed,
   tagged digest.
5. **Concurrency**: one global group, no cancel-in-progress, so "highest
   stable" is evaluated serially.
6. **Permissions**: workflow default `contents: read`; build and promote jobs
   `packages: write`; the smoke job `packages: read` (a pull-only capability);
   the sign job `packages: write` (cosign pushes signature and attestation
   artifacts to GHCR) and `id-token: write`; the release job
   `contents: write`.
7. **Frozen identity**: the signing certificate carries the workflow path, so
   `.github/workflows/release.yml` must not be renamed and signing must remain
   inside it, never in a reusable workflow.
8. **Pinning (DDD-44)**: every `uses:` is pinned to a full commit SHA with the
   version in a comment; cosign, syft and buildx versions are pinned in the
   workflow; base images (`golang`, `node`, `alpine`, and the smoke's
   `postgres:16`) are pinned by digest. An automated check fails the build on
   an unpinned `uses:` in `release.yml` (zizmor, or a grep test). Bumps via
   dependabot/renovate; digest values are DEVOPS's to fill.
9. **Architectures**: both are smoke-tested natively (free `ubuntu-24.04-arm` for
   a public repo, QEMU as fallback; DEVOPS finalises).
10. **Harnesses (DDD-46)**: the repository's one existing scripts convention is
    a Go program per directory (`scripts/race/main.go`). The smoke harness
    (`scripts/smoke/`) and the quickstart checker (`scripts/quickstart/`, which
    extracts the tagged fenced command blocks of
    `docs/quickstart-published-image.md` and runs them in order, so the document
    is the tested artifact) follow it and share helpers; both run in
    `release.yml` and locally.
11. **First release procedure**: the first push creates a private package, so
    the pipeline's step 6 fails on the first run by design. The owner sets the
    package public (OQ-7), then re-runs; the run builds anew (the tag does not
    exist yet).

## Alternatives considered

- **Literal D8 order (push tagged, smoke, sign, then latest).** An unsmoked,
  unsigned `X.Y.Z` is publicly pullable meanwhile and a failed smoke leaves a
  public version tag. Rejected; the smoke still tests registry bytes.
- **Anonymous probe after tag promotion.** A private package would already
  carry a public-looking tag and a partial release. Rejected; moved before it.
- **Reproducible builds so a re-run yields the same digest.** Large effort
  (timestamps, buildx rewrite-timestamp, base image drift) for little value
  against the resume rule. Rejected.
- **Scheduled prune of untagged versions.** Breaks multi-arch indexes as
  described. Rejected for v0.1.0.
- **`workflow_run` / polling for CI on the same SHA.** Fragile, racy. Rejected.
- **Inline shell for "highest stable".** Untestable business rule (US-6).
  `docker/metadata-action` alone cannot express the backport rule. Rejected.
- **Shell scripts for the smoke harness.** Departs from the repository's Go
  scripts convention and from the reusable testcontainers dependency. Rejected.

## Consequences

- Residual risk, documented: GHCR cannot enforce tag immutability. The workflow
  guard stops the project overwriting a tag; anyone with write access could.
  Consumers should pin by digest; cosign proves who built a digest, not what a
  tag points at.
- The first release needs one failed-by-design run; documented in the
  DEVOPS handoff.
- Unpruned orphan digests accumulate slowly; acceptable for v0.1.0.

## Amendment: revision check on the resume path (2026-10-19, user decision; DDD-45)

Decision 4 resumes with the existing digest when `X.Y.Z` is already in GHCR. The run now also reads that image's `org.opencontainers.image.revision` label and compares it to the commit being released; a mismatch fails the run with a message naming both commits and the digest, and nothing is promoted or signed. This catches a moved git tag, which the tag-absent-or-same-digest guard (decision 2 step 7) cannot, because the registry tag still points at the old, correct digest. The label is written by `docker/metadata-action` at build, so it is the same value the US-6 scenario already asserts. A deliberately moved tag therefore requires a new version, consistent with "published versions are never overwritten".

## Amendments from DEVOPS (2026-10-19; `devops/upstream-changes.md` items 1, 5, 7, 9, 10, 11)

The text above is kept as decided; these amend it. Decisions DDD-49..51 record them.
- **First release (item 1, user-decided 2026-10-17):** the first image release is `v0.2.0` (release candidates `v0.2.0-rc.N`); the source-only tag `v0.1.0` stays untouched. `classify` lists versions from GHCR, never from git tags, so `v0.1.0` is invisible to it. Where this ADR says "v0.1.0" (the pruning and first-release remarks), read `v0.2.0`.
- **Build topology (item 5):** decision 2 step 3 becomes one multi-platform build that pushes the untagged index by digest: no per-platform matrix and no merge job. Nothing executes on arm64 during build (ADR-019), so only the smoke needs a native arm64 runner. BuildKit provenance and SBOM attestations are turned off so the index holds exactly the declared platforms; the cosign SBOM attestations (ADR-021) are unaffected.
- **Re-run wording (item 9):** decision 11 "the re-run builds anew" applies to a fresh run with the tag absent (re-push, or "Re-run all jobs"). GitHub's "Re-run failed jobs" replays the same run and reuses earlier job outputs, so the earlier digest is reused. DDD-45 is unchanged (tag exists: resume with its digest; absent: build anew). Every run prints `image_digest=... source=built|reused-output|resumed-from-tag`.
- **Stale-plan guard (item 9, replaces the "re-evaluating `moveLatest` at that moment" wording of decision 4):** `classify` runs the pure planner once and passes the plan; `promote` (no checkout, no repository code, `packages: write`) fails with "plan stale, re-run all jobs" when the highest stable tag in GHCR differs from the plan's `highest_stable`. Equivalent safety; the write-permission job stays small and the rule stays in the tested planner.
- **Concurrency caveat (item 10):** GitHub keeps one running and one pending run per concurrency group and cancels an older pending run when a newer one queues. The global group is kept for serialised "highest stable"; the mitigation is operational (a notice in `classify`, the runbook), not architectural.
- **`ci` gates every path (item 11):** `ci` is in the `needs` of every job after `classify`, including the resume path; downstream jobs accept `build` being skipped. `anon-probe` and `promote` also need `sign` once slice 07 lands. Because `!cancelled()` disables the implicit `success()`, every job's `if:` names each dependency explicitly. A `workflow-guards` structural test asserts that (`if_clauses_complete`). Disagreement noted, not blocking: explicit `if:` lists are brittle to edit, so the test, not review discipline, is the control.
- **Tag namespace (item 7):** GHCR answers 404 to the OCI referrers API for public digests, so cosign stores signatures and attestations as tags `sha256-<hex>.sig` and `.att` in the same package. The `internal/release` classifier's "existing versions" input ignores every tag that is not strict semver or `latest` (signature-tag noise is added to its table tests), and the manual orphan cleanup treats a `.sig`/`.att` version as belonging to its subject digest.
