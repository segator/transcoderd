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
- **THEN** the dashboard shows its original filename, downloaded bytes and total size, transfer speed, elapsed time and ETA when available

#### Scenario: Assigned job waiting to download
- **WHEN** an assigned job has not started downloading its source
- **THEN** the dashboard shows the source filename and identifies that it is waiting for download

#### Scenario: FFmpeg encoding progress
- **WHEN** FFmpeg reports encoding progress
- **THEN** the dashboard shows encoded frames, frame rate, encoder speed, elapsed time and ETA when available

#### Scenario: Subtitle preparation
- **WHEN** a job is extracting subtitle tracks or converting PGS subtitles with OCR
- **THEN** the dashboard identifies the active phase as subtitle extraction or subtitle OCR instead of generic preparation

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

### Requirement: Configurable Windows worker working directory
The desktop settings SHALL let a user select an absolute path on a local Windows drive for downloads and job workspaces. If no path is selected, the service SHALL use `%ProgramData%\TranscoderD\work`. Both worker profiles SHALL use the selected directory as the parent of their own workspaces.

#### Scenario: Change working directory
- **WHEN** a user saves a different valid working directory
- **THEN** the service applies it after accepted work drains, and both worker profiles store downloads and job workspaces beneath that directory

### Requirement: Independent CPU and GPU worker profiles
The Windows dashboard SHALL let users configure separate encoder and concurrent-job limits for CPU and GPU profiles under one service.

#### Scenario: Mixed CPU and GPU configuration
- **WHEN** the user configures one CPU encode and four GPU encodes
- **THEN** the service runs both profiles concurrently with those independent limits

#### Scenario: Disable one profile
- **WHEN** the user sets one profile's concurrency to zero and the other to a positive value
- **THEN** the service accepts jobs only through the enabled profile

#### Scenario: Migrate a saved single-profile configuration
- **WHEN** an existing installation has one saved encoder and concurrency limit
- **THEN** the service maps that configuration to its matching CPU or GPU profile and preserves its behavior

### Requirement: Shared queue assignment across encoder profiles
CPU and GPU profiles SHALL request work independently from the existing server queue, and each assigned job SHALL use the encoder profile that received it.

#### Scenario: Both profiles request queued work
- **WHEN** CPU and GPU profiles have available capacity and jobs are queued
- **THEN** either profile may receive each job according to which request the server handles first

### Requirement: Aggregated mixed-profile monitoring
The dashboard SHALL aggregate active jobs, progress, encode counts, and admission state across enabled CPU and GPU profiles while identifying each job's profile.

#### Scenario: Display simultaneous mixed-profile work
- **WHEN** CPU and GPU profiles encode jobs at the same time
- **THEN** the dashboard shows both jobs and their respective profiles and includes both in aggregate counts

#### Scenario: Pause and resume mixed profiles
- **WHEN** the user pauses or resumes the worker
- **THEN** new assignments stop or resume for both profiles while already accepted work continues

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
