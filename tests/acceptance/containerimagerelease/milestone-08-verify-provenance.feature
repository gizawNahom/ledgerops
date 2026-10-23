@US-7 @slice-07 @env-ghcr-public-package
Feature: Verify the image came from the project's pipeline

  @pending @error @kpi @KPI-R5 @contract-shape:unbounded-preservation
  Scenario: The published identity pattern matches only this project's release workflow
    Then the published identity matches "https://github.com/gizawNahom/ledgerops/.github/workflows/release.yml@refs/tags/v0.2.0" and "https://github.com/gizawNahom/ledgerops/.github/workflows/release.yml@refs/tags/v0.3.0-rc.1"
    And the published identity does not match "https://github.com/gizawnahom/ledgerops/.github/workflows/release.yml@refs/tags/v0.2.0" and "https://github.com/gizawNahom/ledgerops/.github/workflows/release.yml@refs/heads/main" and "https://github.com/gizawNahom/ledgerops/.github/workflows/other.yml@refs/tags/v0.2.0" and "https://github.com/someone-else/ledgerops/.github/workflows/release.yml@refs/tags/v0.2.0"

  @pending @requires_external @kpi @KPI-R5 @contract-shape:unbounded-preservation
  Scenario: A released image verifies against the project's identity
    Given the real release pipeline has published "0.2.0-rc.1"
    Then cosign verifies "ghcr.io/gizawnahom/ledgerops:0.2.0-rc.1" against the project identity

  @pending @requires_external @error @kpi @KPI-R5 @contract-shape:unbounded-preservation
  Scenario: A look-alike image fails verification
    Given the real release pipeline has published "0.2.0-rc.1"
    Then cosign does not verify "ghcr.io/gizawnahom/look-alike:0.2.0-rc.1" against the project identity

  @pending @requires_external @kpi @KPI-R5 @contract-shape:unbounded-preservation
  Scenario: The software bill of materials is retrievable per platform
    Given the real release pipeline has published "0.2.0-rc.1"
    Then the SBOM attestation of the amd64 digest of "0.2.0-rc.1" verifies

  @pending @requires_external @kpi @KPI-R5 @contract-shape:unbounded-preservation
  Scenario: The first release candidate's real certificate matches the published identity
    Given the real release pipeline has published "0.2.0-rc.1"
    Then cosign verifies "ghcr.io/gizawnahom/ledgerops:0.2.0-rc.1" against the project identity
