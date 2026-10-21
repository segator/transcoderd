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
With updates enabled, the supervisor SHALL select the release asset whose name matches the running server or worker binary, download it to temporary storage, and apply it to the executable before running the application.

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
