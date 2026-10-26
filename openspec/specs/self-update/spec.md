# Binary self-update

## Purpose

Define how server and worker binaries discover newer project releases, honor the update settings, and replace their executables when automatic updates are enabled.

## Requirements

### Requirement: Release comparison
Each process SHALL query the project's latest GitHub release and compare its semantic version with the running version, subject to `updateCheckInterval`, whose registered default is 15 minutes.

#### Scenario: Newer release
- **WHEN** the latest release has a greater semantic version
- **THEN** the process identifies an available update

#### Scenario: Check interval not elapsed
- **WHEN** the last successful query is still within `updateCheckInterval`
- **THEN** another remote query is not made

### Requirement: Updates disabled
The `noUpdates` setting SHALL prevent executable replacement even when a newer release exists.

#### Scenario: New release with updates disabled
- **WHEN** `noUpdates` is true and a newer release is found
- **THEN** the process logs the availability and continues without applying it

### Requirement: Supervisor replacement
With updates enabled, the supervisor SHALL select the release asset corresponding to the running application and platform, download it to temporary storage, and apply it to the executable before running the application.

#### Scenario: Matching asset exists
- **WHEN** a newer release contains the corresponding binary asset
- **THEN** the supervisor downloads and applies it before starting the service

#### Scenario: Matching asset absent
- **WHEN** the release has no matching asset
- **THEN** the update reports an error instead of applying an unrelated asset

### Requirement: Update checks during service
The worker SHALL check for an update before requesting a new job when it is ready to accept work. The server SHALL check periodically while no HTTP requests are active.

#### Scenario: Server has active requests
- **WHEN** the server's periodic check is due but an HTTP request is active
- **THEN** it skips that check

#### Scenario: Worker is ready for work
- **WHEN** the worker reaches a job-request cycle and accepts new work
- **THEN** it checks for an update before asking for an assignment

### Requirement: Platform-specific worker updates
Windows x64 workers SHALL select transcoderd-worker-windows-amd64.exe for binary updates and SHALL never apply the Linux executable. Existing Linux amd64 release asset names SHALL remain compatible. Binary self-updates SHALL preserve bundled tools; upgrading the toolchain SHALL require replacing the complete bundle.

#### Scenario: Mixed-platform release
- **WHEN** a release contains Linux and Windows worker executables
- **THEN** a Windows x64 worker selects only the Windows x64 executable

#### Scenario: Windows asset absent
- **WHEN** the newer release has no matching Windows executable
- **THEN** no incompatible executable is installed

#### Scenario: Repeated updates with a running supervisor
- **WHEN** the Windows supervisor applies another binary update while its original executable remains mapped
- **THEN** the new executable is installed without colliding with the previous update's backup
