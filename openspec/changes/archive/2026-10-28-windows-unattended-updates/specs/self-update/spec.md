# Spec Delta

## ADDED Requirements

### Requirement: Mandatory managed Windows updates
The Windows desktop service SHALL update its complete installation automatically, independently of interactive logon. It SHALL check stable releases at startup and every hour with up to five minutes of jitter. Installation SHALL require no user confirmation and SHALL provide no deferral or opt-out setting. An explicit check SHALL also be available.

#### Scenario: Unattended automatic update
- **WHEN** a newer eligible stable release is available
- **THEN** the service prepares the update without requiring the tray or a user's confirmation

#### Scenario: Worker is paused or outside its schedule
- **WHEN** a release check is due while worker admission is paused or outside its schedule
- **THEN** checking and updating proceed independently of the job admission window

#### Scenario: User attempts to postpone an update
- **WHEN** a local client requests confirmation-only, disabled or deferred updates
- **THEN** the service rejects that request and continues the mandatory update lifecycle

### Requirement: Authenticated complete Windows update
The service SHALL install only a newer stable Windows x64 installer from the project's releases whose metadata signature verifies against a trusted application key. It SHALL validate the release identity, installer size and digest before execution. Missing release assets or invalid metadata SHALL not establish a pending update. Once an eligible update is pending, download or storage failures SHALL retain that pending update and its admission block.

#### Scenario: Release is published before Windows assets
- **WHEN** the latest release lacks complete signed Windows update assets and no eligible update is already pending
- **THEN** the service continues its current work and retries later without establishing an admission block

#### Scenario: Tampered download
- **WHEN** the downloaded installer differs from its authenticated size or digest
- **THEN** it is not executed, the dashboard reports the failure and admission stays blocked while the service retries the authenticated pending update

#### Scenario: Unsupported release
- **WHEN** the release is a draft, prerelease, incompatible architecture or does not have a greater semantic version
- **THEN** it is not selected for automatic installation

### Requirement: Immediate Windows update admission barrier
After authenticating a complete eligible release, the service SHALL persist the pending update and immediately stop new job requests on both profiles. It SHALL download the installer while accepted work finishes and install as soon as the installer is verified and all work drains. There SHALL be no confirmation or grace period. The admission block SHALL survive service and machine restarts until the installed release is verified current.

#### Scenario: Installer is still downloading
- **WHEN** a complete eligible release has been authenticated but its installer is still downloading
- **THEN** both profiles stop initiating job requests and continue only accepted work and requests already in flight

#### Scenario: Service restarts with a pending update
- **WHEN** the service starts with a persisted pending update
- **THEN** admission remains blocked before either worker profile can request new work and updating resumes automatically

### Requirement: Drain all accepted Windows work
Before replacing installation files, the service SHALL stop new assignments locally for both profiles and wait until all accepted jobs and reserved requests finish. Downloads, subtitle work, queued encodes, verification and uploads SHALL complete normally. An unreachable profile SHALL not be treated as idle. Maintenance draining SHALL preserve persisted server job limits and the user's pause and schedule settings.

#### Scenario: Encode finishes while its upload is running
- **WHEN** encoding has completed but an accepted upload remains active
- **THEN** the installer is not started until that upload and all other accepted work finish

#### Scenario: Both profiles are busy
- **WHEN** CPU and GPU profiles have accepted work at update time
- **THEN** neither receives further assignments and installation waits for both profiles and reserved requests to drain

#### Scenario: Existing pause or schedule
- **WHEN** an update completes for a paused worker or outside its admission schedule
- **THEN** the service retains that pause or schedule and does not admit jobs contrary to it

### Requirement: Recoverable Windows installation transaction
Complete Windows upgrades SHALL use one durable installation transaction that survives service termination and machine restart. The system SHALL keep a recoverable previous installation until installed files and service health match the intended release. Failed installation or health validation SHALL restore the previous installation and configuration while retaining the pending update and admission block. Unsafe recovery SHALL remain stopped and report its error.

#### Scenario: Installer failure
- **WHEN** the installer fails after replacing some installation files
- **THEN** recovery restores the previous complete installation and retained configuration, keeps admission blocked and retries updating automatically

#### Scenario: Machine restarts during installation
- **WHEN** Windows restarts with an incomplete transaction
- **THEN** recovery finishes verification or restores the previous installation, and workers accept new work only after the mandatory update is successfully verified

#### Scenario: Concurrent installation requests
- **WHEN** automatic progress and explicit check requests overlap
- **THEN** only one installation transaction runs

### Requirement: Automatic mandatory update retries
Failed checks, staging and installation SHALL retry automatically with bounded backoff, using delays of 15 minutes, one hour and then at most six hours. The service SHALL report the failure and next retry without requiring user action. An authenticated pending update SHALL keep admission blocked throughout retries; a newer authenticated eligible release SHALL supersede the failed target.

#### Scenario: Repeated release failure
- **WHEN** staging, installation or post-install validation fails repeatedly
- **THEN** recovery and retries continue automatically at the bounded intervals while the dashboard explains why the worker is not accepting jobs

#### Scenario: A fixed release supersedes a failing version
- **WHEN** a newer eligible release becomes available during update retries
- **THEN** the service automatically selects it and retains the admission block until successful installation and verification
