## ADDED Requirements

### Requirement: Complete an existing stable Windows release
Release automation SHALL support completing an existing stable release from its immutable tagged source and exact version. Completion SHALL preserve existing Linux artifacts and pass all ordinary Windows validation gates before publishing. It SHALL reject draft, prerelease, mismatched-version and non-mainline sources and SHALL recheck tag identity before publication.

#### Scenario: Recover missing Windows distribution
- **WHEN** an operator dispatches completion from main for an existing stable release whose tag matches its version and mainline source
- **THEN** the workflow builds and validates that exact source and version before uploading its Windows assets
- **AND** the tag and existing Linux distribution remain unchanged

#### Scenario: Reject inconsistent source
- **WHEN** the release is not stable, its version differs from tagged source, its commit is outside main, or its tag moves before publication
- **THEN** completion fails without publishing signed Windows metadata

### Requirement: Publish authenticated update metadata last
Windows release publication SHALL upload validated payloads before publishing their manifest and publish the signature last. Completion SHALL refuse to replace an already signed Windows release.

#### Scenario: Interrupted unsigned publication
- **WHEN** upload stops before the signature is published
- **THEN** the release remains ineligible for automatic Windows updates and completion can retry unsigned assets

#### Scenario: Existing signed release
- **WHEN** completion targets a release that already contains its update signature
- **THEN** publication refuses to replace its Windows payloads
