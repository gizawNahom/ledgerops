@US-3 @US-6 @slice-03 @slice-06
Feature: The right tags for every release

  The release plan is read from the real `releaseplan` program (the driving
  port the workflow executes). The classifier's universal rules are
  property-based unit tests in internal/release (Mandate 9: layer 1).

  @pending @driving_port @contract-shape:pure-function
  Scenario: The first stable release gets exact, minor and latest tags
    Given no versions have been published yet
    When the tag "v0.2.0" is pushed
    Then the plan publishes "0.2.0" and "0.2" and "latest"
    And latest moves
    And the highest stable version is ""

  @pending @driving_port @contract-shape:pure-function
  Scenario: A prerelease never moves latest or the minor tag
    Given the published tags are "0.2.0" and "0.2" and "latest"
    When the tag "v0.3.0-rc.1" is pushed
    Then the plan publishes "0.3.0-rc.1"
    And latest does not move
    And the highest stable version is "0.2.0"

  @pending @driving_port @contract-shape:pure-function
  Scenario: A patch moves the minor tag but keeps the old exact tag
    Given the published tags are "0.2.0" and "0.2" and "latest"
    When the tag "v0.2.1" is pushed
    Then the plan publishes "0.2.1" and "0.2" and "latest"

  @pending @driving_port @contract-shape:pure-function
  Scenario: A backport moves its minor tag but not latest
    Given the published tags are "0.2.0" and "0.2.1" and "0.2" and "0.3.0" and "0.3" and "latest"
    When the tag "v0.2.2" is pushed
    Then the plan publishes "0.2.2" and "0.2"
    And latest does not move

  @pending @driving_port @contract-shape:pure-function
  Scenario: Signature and attestation tags in the package are not versions
    Given the published tags are "0.2.0" and "sha256-ab12cd34ef56.sig" and "sha256-ab12cd34ef56.att" and "latest"
    When the tag "v0.2.1" is pushed
    Then the plan publishes "0.2.1" and "0.2" and "latest"
    And the highest stable version is "0.2.0"

  @pending @driving_port @contract-shape:pure-function
  Scenario: Re-running a published version resumes it
    Given the published tags are "0.2.0" and "0.2" and "latest"
    When the tag "v0.2.0" is pushed
    Then the run resumes the published version
    And latest does not move

  @pending @error @driving_port @contract-shape:pure-function
  Scenario Outline: Only strict version tags are release tags
    Given no versions have been published yet
    When the tag "<tag>" is pushed
    Then the tag is rejected as not a version

    Examples:
      | tag                 |
      | v1.2                |
      | 1.2.3               |
      | v1.2.3.4            |
      | nightly-2026-10-17  |
