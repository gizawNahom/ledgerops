# Upstream changes requested by DESIGN — container-image-release

For the product owner to review. DESIGN did not edit DISCUSS sections. Rationale: `feature-delta.md` § Wave: DESIGN / [REF] Changed Assumptions.

1. **US-3, scenario "A published image is proven usable"** (and its AC "After publishing, a check pulls the published image...").
   - Current: "Given `0.1.0` is published / When the pipeline's final check pulls that image, migrates a fresh Postgres and starts it".
   - Requested: the check pulls the pushed image by digest before any version tag exists, for both architectures; the version tag appears only after smoke and signature pass. Suggested wording: "Given the image for `0.1.0` has been pushed by digest / When the pipeline's check pulls that digest ... Then `/health` answers 200 and only then is `0.1.0` tagged".
2. **US-3 scenario "A version that fails its usability check is not presented as a release"**: add "and no version tag exists for it".
3. **US-1 / Gherkin Rule A "A crash partway through a fresh migrate"**: qualify "killed after the first schema change" as "killed between two migration files". Add a sibling scenario: "killed during a migration file leaves a dirty version; re-running migrate refuses naming the version, changes nothing, and `ledgerops_app` still refuses `app-secret`".
4. **US-2**: add a scenario and AC: "`serve` refuses to start when `LEDGEROPS_MIGRATE_DSN` is present in its environment, naming the variable, before any database connection".
5. **US-1 Technical Notes / US-4**: record that `migrate` now runs a preflight privilege probe; an insufficient privilege fails before any schema change with "no changes applied" (tightens US-4's "says whether schema changes were already applied" to a definite answer in the preflight case).
6. **US-1 scenario 2 (secret file)**: add the edge "file not readable by the non-root image user" yields a named error (documented trap for `0400 root`).
7. **US-7 AC "Both architectures are covered (OQ-10 settles how)"**: settled: the index and each platform manifest are signed (`--recursive`) and each platform carries its own SBOM attestation. Informational; no wording change required beyond removing the OQ reference.
8. **US-8 / README**: the verify command uses the identity regex in ADR-021 and the document must state the pin-by-digest recommendation (registry cannot enforce tag immutability) and the restart-after-rotation order (ADR-023).

## Applied by PO 2026-10-17

1. Applied. US-3 scenario and AC reworded to digest-first smoke, version tag only after it passes; slice 03 brief updated. Clarification (partial disagreement, flagged): "for both architectures" is written as "each published architecture", amd64 in slice 03 and arm64 joining in slice 06, because arm64 is a slice 06 deliverable. Also added "and, once slice 07 lands, after signature verification".
2. Applied. Failure scenario and AC now end "no version tag exists for it" (feature-delta US-3, slice 03).
3. Applied. US-1 crash scenario now "killed between two migration files"; new sibling scenario and AC for the dirty-version case; Gherkin [HOW] section and slice 01 and the journey error paths updated.
4. Applied. New US-2 scenario and AC for `LEDGEROPS_MIGRATE_DSN` in the serve environment; Gherkin [HOW], slice 02 and journey updated.
5. Applied. US-1 Technical Notes, US-4 scenario, AC and notes, Gherkin [HOW] and slices 01 and 04 now say preflight probe, "no changes applied".
6. Applied. New US-1 secret-file edge scenario and AC (root 0400 unreadable by non-root user), Gherkin [HOW], slice 01 and journey.
7. Applied. US-7 AC and technical note now state index and each platform signed, per-platform SBOM; OQ-10 references removed (feature-delta, slice 07). The Pre-requisites list entry for OQ-10 in the DISCUSS section is left as a historical record.
8. Applied. US-8 gets a pin-by-digest scenario and AC, ADR-021 identity regex in the verify AC, and the migrate-then-restart order; slice 08 updated.

Side effect: scenario counts are now US-1 8 and US-2 9 (over the 3-7 guideline); recorded as an exception in the DoR section.

## New in review cycle 1 (2026-10-17), for PO

Call-site counts that earlier DESIGN notes cited were checked against the code: `Migrate`/`MigrateStep` have 7 direct call sites in 6 files (not 7 files, and `ledger_seeding.go` has 2, not 4). Items 1-8 above contain no counts, so nothing there changes; the corrected counts are in `feature-delta.md` § Changed Assumptions 3 and ADR-017.

9. **US-7 scenario "The software bill of materials can be retrieved" and US-7 AC "An SBOM attestation is published for every release"; US-8 verification section** (finding B2, ADR-021, DDD-43). SBOMs are attested per platform digest, so they cannot be verified through the tag. Requested wording: "Given release `0.2.0` / When Tomasz resolves the digest for his platform and fetches that digest's SBOM attestation / Then it lists the Go modules and base-image packages for that platform". The index signature still verifies on the tag (`cosign verify image:tag`); only `verify-attestation` targets the platform digest. US-8's quickstart verify section must show both commands (digest lookup, then `verify-attestation`). KPI K5's "`verify-attestation`" wording is then "for each published platform digest".
10. **US-1, one ledgerops database per Postgres cluster** (user decision, DDD-41, ADR-017).
    - Qualify scenario "A database with any name can be prepared" (and the matching AC): "any name" holds for the first ledgerops database in a cluster.
    - Add a scenario and AC: "Given a Postgres cluster whose database `kestrel_ledger` is already migrated (role `ledgerops_app` exists) / When Priya runs `migrate` against a second, empty database `kestrel_ledger_2` in that cluster / Then it exits non-zero naming the conflict, and the role, its password, `kestrel_ledger`, and `kestrel_ledger_2` are all unchanged". A database that already carries the ledgerops schema is not refused (upgrade, rotation, re-run).
    - Journey `pull-and-run-ledgerops.yaml` error path S2 and the documentation (slice 08) state the constraint and the remedy (use a separate cluster, or migrate the database that already owns the role).
11. **US-3 and US-6, GitHub Release content** (finding M10). Add to the "Each tag has a GitHub Release" AC: the release body names the image reference (`ghcr.io/gizawnahom/ledgerops:X.Y.Z`), its digest and the verify command, followed by auto-generated notes.
12. **US-3, anonymous pull and the first release** (finding M13, ADR-020). The check that the package is publicly pullable now runs on the digest before any version tag is promoted. Consequence for the AC "pullable without logging in": the first release run is expected to fail at that step until the owner sets the GHCR package public (OQ-7); the owner then re-runs. The DoD item "A real rc tag (`v0.1.0-rc.1`) has been published" should say so.
14. **Review cycle 2 (N1, N3; DDD-48).** (a) The US-1 cluster-rule scenario from item 10 gains: "And the message names the remedy: migrate the database that owns the role, use a separate cluster, or, when that database no longer exists, run `DROP ROLE ledgerops_app` as a privileged role". (b) Add a US-1 scenario: "Given two empty databases in one cluster / When two `migrate` runs start at the same time / Then exactly one succeeds, the other refuses with the cluster-rule message, and neither database is left in a dirty migration state". (c) US-8 documentation states the `DROP ROLE ledgerops_app` remedy for a dropped owning database or an abandoned first run.
13. **US-8 documentation scope** (findings m5, m6, M2). The quickstart and README must: steer secrets to `LEDGEROPS_APP_PASSWORD_FILE` (environment variables are visible through `docker inspect`); state that under `log_statement=ddl` the password verifier is logged and is offline-attackable, so use a generated high-entropy password; document the manual dirty-migration recovery (check whether migration N's objects exist, then set `schema_migrations` to N-1 or N with `dirty = false`, then re-run `migrate`); and state the one-ledgerops-database-per-cluster constraint (item 10).

## Applied by PO 2026-10-17 (review cycle 1)

9. Applied. US-7 SBOM scenario, AC, KPI measured-by wording and slice 07 now use per-platform-digest attestation with the index signature verifying on the tag; US-8 gets an AC showing both commands (slice 08).
10. Applied. US-1 "any name" scenario and AC qualified to the first database in a cluster; new second-database-refused scenario and AC (an already-migrated database is not refused); technical note, slice 01, and journey S2 error path with remedy; documentation covered by item 13's AC.
11. Applied to US-6 (feature-delta scenario and AC, slice 06). Partial disagreement, flagged: US-3 has no GitHub Release scenario or AC (the Release is slice 06 scope), so nothing to amend there.
12. Applied. US-3 gets an AC for anonymous pull checked on the digest before tagging, with the first-run-expected-to-fail note; DoD item 6 and slice 03 updated.
13. Applied. US-8 gets four ACs (password file, verifier-logging warning, dirty-migration recovery, one database per cluster) and one scenario; slice 08 updated.

Counts: US-1 now has 9 scenarios (exception recorded in the DoR section); US-8 has 7.

## Applied by PO 2026-10-17 (review cycle 2)

14. Applied. (a) US-1 cluster-rule scenario gains the remedy line incl. privileged `DROP ROLE ledgerops_app`; AC and technical note updated. (b) New US-1 concurrency scenario and AC (exactly one succeeds, neither dirty). (c) US-8 AC names the `DROP ROLE` remedy. Slices 01 and 08 and the journey S2 error path updated. US-1 now has 10 scenarios, US-8 still 7.
