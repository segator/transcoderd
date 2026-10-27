# Worker Dashboard Specification

## Purpose

Provide a responsive browser dashboard for monitoring and controlling a running TranscoderD worker on Linux and in Docker.

## ADDED Requirements

### Requirement: Optional authenticated browser dashboard
The worker SHALL serve its dashboard and local status API over HTTP only when a dashboard listen address and bearer token are configured. Requests to the status and control API SHALL provide the configured token. The listener SHALL be disabled by default.

#### Scenario: Dashboard is not configured
- **WHEN** the worker starts without a dashboard listen address
- **THEN** it does not open an HTTP listener

#### Scenario: Authorized dashboard client
- **WHEN** a browser supplies the configured bearer token
- **THEN** it can load worker status and use supported admission controls

#### Scenario: Missing or invalid token
- **WHEN** a client requests a protected API without the configured token
- **THEN** the API returns HTTP 401 and does not expose worker status

### Requirement: Local worker dashboard status
The dashboard API SHALL report the running application version, worker admission state, current phases, job progress, and encode limits from the local worker control state.

#### Scenario: View running worker
- **WHEN** an authorized client loads the dashboard
- **THEN** it sees the installed version and the worker's current local status without requiring a central-server queue endpoint

### Requirement: Worker admission control
The dashboard API SHALL allow an authorized client to pause or resume new job admission using the worker's existing control interface. Existing accepted jobs SHALL continue.

#### Scenario: Pause assignments
- **WHEN** an authorized client pauses the worker
- **THEN** new jobs are not requested and accepted jobs continue

### Requirement: Prominent activity identity and filters
The dashboard SHALL display each local job's CPU/GPU class and encoder as a visually prominent badge when known. It SHALL provide selectable activity filters that can be combined for encoding, downloading, uploading, and preparation phases, plus an all-activity option.

#### Scenario: Filter active work
- **WHEN** a user selects one or more activity filters
- **THEN** the dashboard shows only matching local jobs and indicates when no jobs match

#### Scenario: Identify an encoder
- **WHEN** a local job has encoder metadata
- **THEN** its card visibly distinguishes GPU encoders such as NVENC from CPU encoders such as SVT-AV1

### Requirement: Shared browser dashboard assets
The Windows tray and the Linux/Docker browser dashboard SHALL use the same dashboard assets and user-facing English strings.

#### Scenario: Build either platform
- **WHEN** Windows or Linux worker artifacts are built
- **THEN** the dashboard presents the same activity and version information on both platforms
