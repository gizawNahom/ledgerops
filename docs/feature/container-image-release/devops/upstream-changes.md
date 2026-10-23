# Upstream changes requested by DEVOPS — container-image-release

For the architect (DESIGN items) and the product owner (DISCUSS items) to review. DEVOPS did not edit DISCUSS or DESIGN sections or ADRs. Narrative: `feature-delta.md` § Wave: DEVOPS. Facts below were checked on 2026-10-17 against live sources.

Items 1-4 are **user-decided (2026-10-17)** and ready for the architect/PO to apply. Items 5-8 are corrections that follow from facts. Items 9-11 were added in revision cycle 1 (platform review).

1. **User-decided: first image release is `v0.2.0`** (DISCUSS D14, DoD item 6, every story fixture that says `0.1.0`; OPS-25).
   - Original: "D14 First version is `v0.1.0`; release notes are GitHub auto-generated." DoD 6: "A real rc tag (`v0.1.0-rc.1`) has been published".
   - Decided: the existing source-only tag and pre-release `v0.1.0` (commit `71ec50bf659799ecd1d3991bf2942673cc426d6f`, public since 2026-10-16) stay untouched. The first image is `v0.2.0`, rcs `v0.2.0-rc.N`.
   - PO to apply: amend D14 (and the DISCUSS wave-decisions line); DoD 6 to `v0.2.0-rc.1`; story fixtures that use `0.1.0` as the first published image become `0.2.0` (the `0.2.0`/`0.3.0-rc.1` fixtures in US-6 and US-7 then need shifting by one minor, or may stay illustrative). `classify` lists versions from GHCR, never git tags, so `v0.1.0` is invisible to it.

2. **User-decided: cosign identity uses the exact case `gizawNahom`** (ADR-021 item 5; US-7, US-8 verify commands; DDD-38; OPS-24).
   - Original: `^https://github.com/gizawnahom/ledgerops/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+(-.+)?$`.
   - Decided: `^https://github.com/gizawNahom/ledgerops/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+(-.+)?$`, issuer `https://token.actions.githubusercontent.com`. The first rc's `sign` job prints the real certificate identity and asserts the published regex matches it. The image reference stays lowercase (`ghcr.io/gizawnahom/ledgerops`, GHCR requires it).
   - Architect to apply: ADR-021 item 5 text; PO: US-7 and US-8 command text.

3. **User-decided: runtime base `alpine:3.24`** (ADR-019 item 4; Technology choices "alpine:3.20 (existing)"; OPS-17).
   - Reason: Alpine 3.20 ended support 2026-04-01 (alpinelinux.org `releases.json`). Digest `sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6` (index; amd64 and arm64 verified). Architect to apply to ADR-019 and the Technology choices table.

4. **User-decided: Node 22 slim for the image console stage and the CI console job** (ADR-019 item 4 "Node stage"; Technology choices "Node 20 stage, Vite 5"; OPS-17).
   - Found: `npm ci` fails in `node:20-alpine` with `EBADPLATFORM` (`@pact-foundation/pact-core-linux-x64-glibc` rejects musl; reproduced); `@pact-foundation/pact@17.1.2` and `pact-core@20.1.0` declare `node >=22`.
   - Decided: `node:22-slim` (`sha256:c3de60bf2f9dd0ac6370e6117950ff62d6e339527e7472301c9c78a017978392`) and `node-version: "22"` in `ci.yml`. Measured: `npm ci` 61 s cold, `vite build` 3 s. Architect to apply to ADR-019 and the Technology choices table.

5. **Build topology** (ADR-020 item 2 step 3 and DDD-35).
   - Original: "build per platform (amd64, arm64), push by digest with no tag, merge into an untagged index".
   - Requested: one multi-platform build that pushes the untagged index by digest; no per-platform matrix, no merge job. Reason: the Dockerfile cross-compiles on `$BUILDPLATFORM` and the final stage has no `RUN`, so nothing executes on arm64 during build (DDD-34); `workflow-guards` asserts `FROM --platform=$BUILDPLATFORM` on the golang and node stages, `GOOS`/`GOARCH` from `TARGETOS`/`TARGETARCH`, and no `RUN` after the last `FROM`. Only the smoke needs a native arm64 runner. Related: BuildKit provenance and SBOM attestations are turned off so the index holds exactly the declared platforms (ADR-021 item 6 said provenance "may be emitted"; this makes it "not emitted"); the cosign SBOM attestations are unaffected.

6. **SBOM tooling** (ADR-021 item 2, Technology choices "syft").
   - Requested wording only: syft v1.54.1 is installed from its release tarball with a pinned sha256 (`c069905b391cc4c20a5ba65ad5c10be2a7ba074f8ea6ad203e24d14e303dad47` for linux_amd64) instead of through `anchore/sbom-action`. SPDX-JSON per platform digest and `cosign attest --type spdxjson` are unchanged.

7. **Tag namespace** (ADR-020 item 4 re-run semantics).
   - GHCR was probed for the OCI referrers API and answered 404 for an existing public digest, so cosign 2.x will store signatures and attestations as tags `sha256-<hex>.sig` and `.att` in the same package (ADR-021 item 3 already allows this). Requested: the `internal/release` classifier's "existing versions" input ignores any tag that is not strict semver or `latest` (add signature-tag noise to its table tests), and the (manual) orphan cleanup treats a `.sig`/`.att` version as belonging to its subject digest.

8. **Test placement** (DESIGN Component decomposition, Architecture Enforcement).
   - Found: no CI job runs `./cmd/...`, `./internal/support/...` or `./internal/release/...` (the last does not exist yet); the only `internal/adapters/http` test that runs is the Pact provider test (via `-run`), so the package's other tests (five test files in total, filtered by `-run`) run in no CI job. DESIGN's new pure tests and the console-embed tests would have no home.
   - Requested: `unit` gains those three paths; the `console` job runs the whole HTTP adapter package plus a `-tags console_embed` run after copying the built bundle to `console_dist`; structural tests (Dockerfile `FROM` digests and `-tags console_embed`, `release.yml` needs graph) run in a new `workflow-guards` job. A further required job, `container-acceptance` (`needs: [integration]`), runs the `@dedicated-cluster` suites (US-1a, US-1b, US-2, US-4, dirty state, SCRAM, privilege fixtures, console-embed builds) and is the emitter for KPI-R3, R4, R8, R9; the suite path is DISTILL's.

9. **Re-run wording and the `latest` re-evaluation** (ADR-020 item 4 "re-evaluating `moveLatest` at that moment"; item 11 "the re-run builds anew"; OPS-29, OPS-30).
   - Clarify item 11: "builds anew" applies to a fresh run when the tag is absent (re-push, or "Re-run all jobs"); GitHub's "Re-run failed jobs" replays the same run and reuses earlier job outputs, so the earlier digest is reused. DDD-45's rule (tag exists: resume with its digest; absent: build anew) is unchanged. Every run prints `image_digest=… source=built|reused-output|resumed-from-tag`.
   - Item 4's re-evaluation is implemented as: `classify` runs the pure planner once and passes the plan; `promote` (no checkout, no repository code, `packages: write`) fails with "plan stale, re-run all jobs" if the current highest stable GHCR tag differs from the plan's `highest_stable`. Equivalent safety, smaller write-permission job.

10. **Concurrency caveat** (ADR-020 item 5; OPS-27). Add the fact that GitHub holds one running and one pending run per group and cancels an older pending run when a newer one queues; the group is kept for serialised "highest stable". Mitigation is operational (notice in `classify`, runbook), not architectural.

11. **`ci` gates every path, including resume** (ADR-020 item 2 order; OPS-14). `ci` is in the `needs` of every job after `classify`; downstream jobs accept `build` being skipped. `anon-probe` and `promote` also need `sign` from slice 07. Because `!cancelled()` disables the implicit `success()`, each job's `if:` names every dependency explicitly (table in the feature-delta outline). Enforced by a structural test in `workflow-guards` (KPI-R2, field `if_clauses_complete`). Also: a guard that `cmd/...`, `internal/domain` and `internal/app` never import `internal/adapters/postgres/prefeature`.

## Applied by PO 2026-10-19

1. Applied. D14 amended (original text quoted), OQ-12 lines, Wave Decisions Summary and DoD item 6 (`v0.2.0-rc.1`) updated. Fixtures `0.1.0`/`0.1.1`/`v0.1.0-rc.1` became `0.2.0`/`0.2.1`/`v0.2.0-rc.1` in US-2, US-3, US-5, US-8, the Gherkin [HOW] section and slices 03, 07, 08. US-4 and the US-8 upgrade example already used `0.2.0` and now correctly model the first image release, so they are unchanged. US-6: `v0.2.0` as the first stable release and `v0.3.0-rc.1`, `v0.2.1` stay as they are; the backport example `v0.1.9` after `0.2.0` was replaced by `v0.2.2` after `0.3.0` (a `0.1.x` image line cannot exist now), in US-6, the Gherkin [HOW] section and slice 06; the US-6 "no earlier release" Given now says no image has been published and notes that source-only `v0.1.0` does not count.
2. Applied. US-7 verify command carries the full exact-case regex (`gizawNahom`, `release\.yml@refs/tags/v...`); US-8 AC states the exact case and that the image reference stays lowercase; journey S5 notes it.
3-11. Not addressed to the PO (ADR/DESIGN/DEVOPS items); no DISCUSS change. Item 4 (Node 22) and item 3 (alpine 3.24) do not appear in DISCUSS text.

## Changed Assumptions (this wave's own, per the DEVOPS skill)

| Quote | New assumption | Rationale |
|---|---|---|
| ADR-019: "Node stage" / Technology choices: "Node 20 stage, Vite 5 (existing)" | `node:22-slim`, CI console job on Node 22 (user-decided) | item 4 |
| Technology choices: "Base image alpine:3.20 (existing)" | `alpine:3.24` (user-decided) | item 3 |
| ADR-020: "build per platform (amd64, arm64), push by digest with no tag, merge into an untagged index" | one multi-platform build pushed by digest | item 5 |
| ADR-021 item 5: lowercase identity regex | exact case `gizawNahom` (user-decided) | item 2 |
| D14: "First version is `v0.1.0`" | `v0.2.0` (user-decided) | item 1 |
| DESIGN open item 4: "pinned cosign/syft/buildx versions" | cosign v2.6.5, syft v1.54.1 (tarball + sha256), buildx v0.38.0, each verified in the workflow by sha256 or cited installer verification | resolved against real upstreams |
| ADR-020 item 9: "free `ubuntu-24.04-arm` for a public repo, QEMU as fallback; DEVOPS finalises" | native arm64 runner; staged into slice 06; QEMU not built | repository verified public |

## Applied by architect 2026-10-19

Applied as dated amendments ("from DEVOPS"); original text kept. DISCUSS and DEVOPS sections untouched.

1. Item 1 (first release `v0.2.0`): architect side recorded in the ADR-020 amendment and DESIGN Changed Assumptions 6; D14, DoD 6 and story fixtures remain the PO's.
2. Item 2 (exact-case regex): ADR-021 amendment, DDD-38 annotated; US-7/US-8 command text remains the PO's.
3. Item 3 (`alpine:3.24`): ADR-019 amendment, Technology choices, DDD-34.
4. Item 4 (`node:22-slim`): ADR-019 amendment, Technology choices, CI console job row in the component table.
5. Item 5 (build topology, provenance off): ADR-020 and ADR-021 amendments, ADR-019 topology contract, DDD-35 annotated.
6. Item 6 (syft tarball): ADR-021 amendment, Technology choices.
7. Item 7 (tag namespace): ADR-020/021 amendments, new DDD-50 (classifier ignores non-semver tags).
8. Item 8 (test placement): new DDD-51, CI workflow component row, Architecture Enforcement bullet.
9. Item 9 (re-run wording, stale-plan guard): ADR-020 amendment, DDD-45 clarified, new DDD-49.
10. Item 10 (concurrency caveat): ADR-020 amendment, DDD-49.
11. Item 11 (`ci` gates every path, explicit `if:`): ADR-020 amendment, DDD-49, enforcement bullet including the `prefeature` import guard.

Disagreements: none blocking. One note recorded in ADR-020: explicit per-job `if:` lists are brittle to edit, so the `if_clauses_complete` structural test is the control rather than review discipline. Item 9's stale-plan guard replaces my "re-evaluate `moveLatest` at that moment" wording with equivalent safety.

## Not changed (checked, no conflict)

Digest-first order, per-job permissions, frozen `release.yml` path, global non-cancelling concurrency, resume-on-rerun, no automatic pruning, SBOM per platform digest, `latest` last and highest-stable only.
