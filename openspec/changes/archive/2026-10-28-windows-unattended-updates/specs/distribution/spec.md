# Spec Delta

## ADDED Requirements

### Requirement: Authenticated Windows installation releases
Validated Windows releases SHALL publish a signed update manifest binding the stable version, x64 installer identity, installer size and digest, and expected installed-file digests. Automatic clients SHALL trust a pinned verification key. The release signing key SHALL remain in protected CI secrets and SHALL NOT be included in application files, artifacts or logs.

#### Scenario: Windows release assets are complete
- **WHEN** native Windows release validation succeeds
- **THEN** the matching installer and signed manifest are published for that version and are discoverable by installed Windows services

#### Scenario: Windows validation fails
- **WHEN** build, native tests or unattended upgrade validation fails
- **THEN** no signed metadata marks that release eligible for automatic Windows installation

### Requirement: Native unattended Windows upgrade validation
Windows CI SHALL validate mandatory unattended upgrades with SVT-AV1 CPU work, original-owner control-channel access, retained settings and post-install health. It SHALL cover immediate admission blocking, automatic retry after failures and restarts, rollback, and invalid metadata rejection in an isolated runner. The validation SHALL require neither a GPU nor manual dialogs.

#### Scenario: Upgrade during an accepted job
- **WHEN** an update is detected during the fixture's accepted work
- **THEN** job requests stop as soon as the release is authenticated, the fixture finishes including upload, and the complete installation upgrades and resumes with its prior limits without user action

#### Scenario: Upgrade fails or is interrupted
- **WHEN** the test injects an installer failure or an incomplete transaction
- **THEN** recovery preserves owner access, configuration and persisted work, retains the pending admission block and retries automatically until the update succeeds

#### Scenario: Invalid update source
- **WHEN** the test supplies a bad signature, digest, version, architecture or incomplete release asset set
- **THEN** the service rejects installation, establishes no pending update from unauthenticated metadata and retains any already authenticated pending update and admission block

#### Scenario: Slow installer download
- **WHEN** the installer download is held open after complete eligible release metadata has been authenticated
- **THEN** the test observes no new worker requests during the download and accepted work continues normally
