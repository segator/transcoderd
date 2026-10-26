# Windows Desktop Worker Specification

## Purpose

Lets Windows users install, configure, and monitor a native TranscoderD worker through a secure service and a web-based tray dashboard.

## Requirements

### Requirement: Windows desktop worker installation
The distribution SHALL provide an interactive Windows x64 installer that installs the worker, bundled media and OCR tools, a Windows service, and a per-user tray application. It SHALL support upgrades and uninstallation without deleting configuration or persisted work by default.

#### Scenario: First install
- **WHEN** a user completes the installer with administrator privileges
- **THEN** the service is registered for automatic startup and the tray application is available for the installing user

#### Scenario: No connection configured
- **WHEN** the service starts without a server URL and token
- **THEN** it remains available for configuration and does not request work

#### Scenario: Upgrade with accepted work
- **WHEN** an upgrade is requested while jobs are active
- **THEN** the installer SHALL not interrupt those jobs and SHALL require them to finish before replacing binaries

### Requirement: English desktop experience
The Windows installer, tray application, and user-facing service messages SHALL use English.

#### Scenario: First-time setup
- **WHEN** a user installs or configures the worker
- **THEN** the installer, dashboard, and service messages are presented in English

### Requirement: Tray monitoring and settings
The tray application SHALL present an HTML-based dashboard showing service connection and admission status, current job phases and progress, locally queued and downloaded jobs, and the server's queue when the server supports it. It SHALL let the user configure server URL, token, worker name, encoder, concurrent encodes and admission windows. The token SHALL be masked and SHALL NOT be returned to the tray after it has been saved.

#### Scenario: Running encodes
- **WHEN** one or more jobs are encoding
- **THEN** the tray window shows their identifiers, progress and estimated time remaining when available

#### Scenario: Download in progress
- **WHEN** a source download has provided its filename
- **THEN** the dashboard shows that filename and the available download progress

#### Scenario: Prefetched work
- **WHEN** an accepted source has been downloaded and is waiting for an encode slot
- **THEN** the dashboard identifies it as ready to encode separately from active encodes and downloads

#### Scenario: Older server
- **WHEN** the connected server does not expose its global queue view
- **THEN** the dashboard reports that the server queue is unavailable while continuing to display local jobs

#### Scenario: Edit settings
- **WHEN** a user saves valid settings
- **THEN** the service stores them securely, applies a concurrency change without interrupting active work, and applies connection or schedule changes after accepted work drains

#### Scenario: Invalid settings
- **WHEN** a URL, limit, encoder or schedule is invalid
- **THEN** the service reports the error and keeps the active configuration

#### Scenario: Closing the window
- **WHEN** the user closes the tray window
- **THEN** the application remains in the notification area and the worker service continues

### Requirement: Restricted local administration
The service SHALL restrict its control channel to the installing Windows user, SYSTEM and administrators. It SHALL restrict configuration files containing the server token to SYSTEM and administrators. It SHALL NOT expose a network management port.

#### Scenario: Another Windows user connects
- **WHEN** an unrelated user attempts to access the local service pipe
- **THEN** Windows denies access

#### Scenario: A different process impersonates the service pipe
- **WHEN** the tray connects to a pipe that is not owned by SYSTEM
- **THEN** it refuses to exchange settings or credentials with that process

#### Scenario: Service starts without an interactive session
- **WHEN** Windows starts the service before any user logs in
- **THEN** the configured worker can run without the tray application

### Requirement: Graceful desktop reconfiguration
Configuration changes requiring worker restart SHALL first stop admission locally without changing the persisted server job limit, then wait for accepted jobs and in-flight requests to finish before restarting. User-requested pause SHALL use the existing persistent admission control.

#### Scenario: Change schedule while an encode runs
- **WHEN** the user saves a different schedule while jobs are active
- **THEN** new requests stop, active jobs finish, and the worker restarts with the new schedule

#### Scenario: Resume a paused worker
- **WHEN** the user sets a positive concurrency after pausing
- **THEN** the service persists and applies that limit using the existing worker control
