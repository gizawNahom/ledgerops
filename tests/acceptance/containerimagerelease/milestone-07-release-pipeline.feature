@US-3 @US-6 @slice-03 @env-release-ci @structural
Feature: Only images that migrate and serve are promoted

  Workflow-guards: the release workflow is read as parsed YAML. Promote and
  smoke logic run against a local registry. GHCR itself is proven on the real
  v0.2.0-rc.1 (@requires_external).

  # --- structure ------------------------------------------------------------------

  @pending @error @kpi @KPI-R2 @contract-shape:pure-function
  Scenario: Only strict version tag pushes start a release
    Then a release starts for "v0.2.0" and "v0.2.0-rc.1" and not for "v1.2" and "nightly-2026-10-17" and "main"
    And the continuous-integration workflow runs on branches and can be called by the release

  @pending @kpi @KPI-R2 @contract-shape:pure-function
  Scenario: Every release job waits for the full test suite, resumed runs included
    Then every job after classify needs the full test suite

  @pending @kpi @KPI-R2 @contract-shape:pure-function
  Scenario: Every job names each of its dependencies in its condition
    Then every job condition begins with not-cancelled and names a success clause per dependency

  @pending @kpi @KPI-R2 @contract-shape:pure-function
  Scenario: Nothing is promoted unless the smoke check and the anonymous pull probe passed
    Then the "promote" job needs "smoke" and "anon-probe" and "ci"
    And the "anon-probe" job needs "smoke" and "ci"

  @pending @error @contract-shape:pure-function
  Scenario: No release page is created unless promotion completed
    Then the "github-release" job needs "promote" and "ci"

  @pending @error @kpi @KPI-R5 @slice-07 @contract-shape:pure-function
  Scenario: A signing failure fails the release and latest does not move
    Then a failing signing step stops the release and cannot be bypassed

  @pending @kpi @KPI-R5 @slice-07 @contract-shape:pure-function
  Scenario: Nothing is promoted unless the image was signed and verified
    Then the "sign" job needs "smoke" and "ci"
    And the "anon-probe" job needs "sign"
    And the "promote" job needs "sign"

  @pending @error @contract-shape:pure-function
  Scenario: Permissions are the least each job needs
    Then only build, sign and promote may write packages, only sign may mint an identity token, and the workflow default is read-only

  @pending @error @contract-shape:pure-function
  Scenario Outline: Every action in every workflow is pinned to a full commit
    Then every action in "<workflow>" is pinned to a full commit SHA

    Examples:
      | workflow                        |
      | .github/workflows/release.yml   |
      | .github/workflows/ci.yml        |
      | .github/workflows/nightly.yml   |
      | .github/workflows/load-test.yml |

  @pending @error @contract-shape:pure-function
  Scenario: Release scripts never interpolate the tag into shell
    Then no release script interpolates workflow expressions directly

  @pending @contract-shape:pure-function
  Scenario: The build pushes one untagged index by digest, and runs report their digest
    Then the build pushes the untagged index by digest without provenance or SBOM attestations
    And every run prints the image digest and where it came from

  @pending @contract-shape:pure-function
  Scenario: Releases are serialised and never cancel one another
    Then the release is serialised and never cancels a run in progress

  @error @contract-shape:pure-function
  Scenario: The image is built from pinned bases, cross-compiled, non-root and without a step in the final stage
    Then every base image of the image build is pinned by digest
    And the image build cross-compiles on the build platform and embeds the console
    And the image runs as a non-root user and has no step after its final base

  @pending @compose @contract-shape:pure-function
  Scenario: The published-image example uses no demo credential
    Then the published-image example uses no demo credential

  @pending @slice-06 @kpi @KPI-R7 @contract-shape:pure-function
  Scenario: The release builds both architectures and labels the image from the tag
    Then the release builds linux/amd64 and linux/arm64 and labels the image with its version, commit and source

  @pending @slice-06 @contract-shape:pure-function
  Scenario: Each release page names the image, its digest and how to verify it
    Then the release page template names the image reference, its digest and the verify command
    And the release job marks prereleases and never lets a backport take the latest badge

  # --- promote against a local registry ----------------------------------------------

  @pending @real-io @adapter-integration @registry @kpi @KPI-R2 @contract-shape:bounded-change
  Scenario: A stable release promotes exact, minor and latest on the built digest
    Given a local registry holding two built images
    When promote runs for "0.2.0" with tags "0.2.0 0.2 latest" moving latest over highest stable ""
    Then the tags "0.2.0" and "0.2" and "latest" resolve to the built image

  @pending @error @real-io @adapter-integration @registry @kpi @KPI-R2 @contract-shape:unbounded-preservation
  Scenario: A published version is never silently overwritten
    Given a local registry holding two built images
    And the tags "0.2.0" are already published on another image
    When promote runs for "0.2.0" with tags "0.2.0 0.2 latest" moving latest over highest stable "0.2.0"
    Then promote refuses
    And the tag "0.2.0" still points at another image
    And no tag "0.2" or "latest" exists

  @pending @error @real-io @adapter-integration @registry @kpi @KPI-R2 @contract-shape:unbounded-preservation
  Scenario: A stale plan is refused before any tag moves
    Given a local registry holding two built images
    And the tags "0.3.0" are already published on another image
    When promote runs for "0.2.1" with tags "0.2.1 0.2 latest" moving latest over highest stable "0.2.0"
    Then promote refuses saying "plan stale, re-run all jobs"
    And no tag "0.2.1" or "latest" exists

  @pending @real-io @adapter-integration @resume-from-tag @registry @contract-shape:bounded-change
  Scenario: A partially promoted release is completed on re-run
    Given a local registry holding two built images
    And the tags "0.2.0" are already published on the built image
    When promote runs for "0.2.0" with tags "0.2.0 0.2 latest" moving latest over highest stable "0.2.0"
    Then the tags "0.2.0" and "0.2" and "latest" resolve to the built image

  @pending @error @real-io @adapter-integration @registry @resume-from-tag @contract-shape:unbounded-preservation
  Scenario: A resumed release whose published image came from another commit is refused
    Given a local registry holding two built images
    And a published version "0.2.0" whose image was built from commit "aaaa111"
    When the release plan runs for the resumed tag "v0.2.0" at commit "bbbb222"
    Then the release plan refuses naming "aaaa111" and "bbbb222"
    And no tag "0.2" or "latest" exists

  @pending @real-io @adapter-integration @registry @kpi @KPI-R1 @contract-shape:bounded-change
  Scenario: The smoke check passes an image that migrates and serves
    Given a local registry holding two built images
    When the smoke check runs against the repository image
    Then the smoke check reports its pull-to-healthy seconds

  @pending @error @real-io @adapter-integration @registry @kpi @KPI-R2 @contract-shape:unbounded-preservation
  Scenario: The smoke check fails an image that cannot migrate a fresh database
    Given a local registry holding two built images
    When the smoke check runs against an unrelated image
    Then the smoke check fails at the migration step
    And no version tag exists for "0.2.0" on the registry

  # --- the real registry ---------------------------------------------------------------

  @pending @requires_external @error @resume-from-tag @contract-shape:unbounded-preservation
  Scenario: A version tag moved to another commit never changes the published image
    Given the real release pipeline has published "0.2.0-rc.1"
    When the tag "v0.2.0-rc.1" is moved to another commit and pushed
    Then the published digest of "0.2.0-rc.1" is unchanged

  @pending @requires_external @slice-06 @kpi @KPI-R7 @contract-shape:unbounded-preservation
  Scenario: The published index lists both architectures with correct labels
    Given the real release pipeline has published "0.2.0-rc.1"
    Then the index of "0.2.0-rc.1" lists linux/amd64 and linux/arm64 and its labels name the release
