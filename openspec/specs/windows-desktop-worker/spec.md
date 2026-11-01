# Windows Desktop Worker Specification

## Purpose

Lets Windows users install, configure, and monitor a native TranscoderD worker through a secure service and a web-based tray dashboard.

## Requirements

### Requirement: Windows desktop worker installation
The distribution SHALL provide interactive and unattended Windows x64 installation of the worker, bundled media and OCR tools, a Windows service, and a per-user tray application. Upgrades SHALL preserve configuration, persisted work and the original owner SID by default, including when initiated by SYSTEM. Uninstallation SHALL preserve configuration and persisted work by default.

#### Scenario: First install
- **WHEN** a user completes the installer with administrator privileges
- **THEN** the service is registered for automatic startup and the tray application is available for the installing user

#### Scenario: No connection configured
- **WHEN** the service starts without a server URL and token
- **THEN** it remains available for configuration and does not request work

#### Scenario: Upgrade with accepted work
- **WHEN** an upgrade is requested while jobs are active
- **THEN** the installer SHALL not interrupt those jobs and SHALL require them to finish before replacing binaries

#### Scenario: Service initiates an unattended upgrade
- **WHEN** SYSTEM installs an eligible update without an interactive user
- **THEN** setup completes without dialogs or credential prompts, restarts the service and retains the original user's control-channel access and login startup entry

#### Scenario: Tray is open during an upgrade
- **WHEN** the updater replaces the tray application and its web assets
- **THEN** it closes the installed tray safely and restores it for the original owner in an active user session, or makes the updated tray available at the next logon

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
- **THEN** new assignments and queued FFmpeg starts stop or resume for both profiles, while active encodes follow the configured pause mode and downstream work finishes

#### Scenario: Configured limits govern startup
- **WHEN** the desktop specifies one CPU and four GPU encodes and the server retains different old limits
- **THEN** both workers use the desktop limits before starting encodes or accepting assignments, with at most one additional pipeline job per enabled profile

### Requirement: Restricted local administration
The service SHALL restrict its control channel to the installing Windows user, SYSTEM and administrators. It SHALL restrict configuration files containing the server token and privileged update state to SYSTEM and administrators. It SHALL NOT expose a network management port or accept arbitrary executable paths, download URLs or command arguments through update controls.

#### Scenario: Another Windows user connects
- **WHEN** an unrelated user attempts to access the local service pipe
- **THEN** Windows denies access

#### Scenario: A different process impersonates the service pipe
- **WHEN** the tray connects to a pipe that is not owned by SYSTEM
- **THEN** it refuses to exchange settings or credentials with that process

#### Scenario: Service starts without an interactive session
- **WHEN** Windows starts the service before any user logs in
- **THEN** the configured worker can run without the tray application

#### Scenario: Update command contains an executable path
- **WHEN** a local client attempts to select an executable, URL or arbitrary installer arguments
- **THEN** the service rejects the request and only accepts the project's authenticated release selection

### Requirement: Graceful desktop reconfiguration
Configuration changes requiring worker restart SHALL first stop admission locally without changing the persisted server job limit, then wait for accepted jobs and in-flight requests to finish before restarting. User-requested admission pause SHALL use persistent admission limits; RAM pause SHALL preserve its independent local preference.

#### Scenario: Change schedule while an encode runs
- **WHEN** the user saves a different schedule while jobs are active
- **THEN** new requests stop, active jobs finish, and the worker restarts with the new schedule

#### Scenario: Resume a paused worker
- **WHEN** the user sets a positive concurrency after pausing
- **THEN** the service persists and applies that limit using the existing worker control

### Requirement: Desktop update monitoring and controls
The English dashboard SHALL show installed and target versions, check result, download progress, accepted jobs preventing installation, installation or recovery result and next retry. It SHALL provide check-now over the restricted service channel without controls to postpone, disable or require confirmation for updates. Tray notifications SHALL be informational and SHALL not be required for automatic progress.

#### Scenario: Update waits for accepted work
- **WHEN** a verified update is draining both profiles
- **THEN** the dashboard shows the target version and identifies the remaining accepted work while its normal progress stays visible

#### Scenario: Upgrade restarts the service
- **WHEN** the dashboard loses its service connection during installation
- **THEN** it presents the known update state and reconnects to display the new installed version or recovery result

#### Scenario: Mandatory update blocks new work
- **WHEN** an eligible update is pending during downloading, draining or recovery
- **THEN** the UI explains that new work is blocked until updating succeeds, shows the remaining work or retry state and provides no postponement or opt-out action

#### Scenario: Visibly locked configuration
- **WHEN** an authenticated update is pending, installing, recovering or retrying
- **THEN** every dashboard page shows a prominent persistent mandatory-update alert with target version and progress, every configuration input and mutation action is visibly disabled, and keyboard navigation or direct service requests cannot change configuration

#### Scenario: Service disconnects during an update
- **WHEN** a locked dashboard loses its service connection or reloads before obtaining authoritative status
- **THEN** configuration remains locked and the update alert remains visible until the service confirms that the update completed successfully

#### Scenario: User changes encoding settings during maintenance
- **WHEN** a mandatory update is pending and a client attempts to change encoder settings or limits
- **THEN** the service rejects the change with an English maintenance message instead of starting another lifecycle transition

### Requirement: Workspace survives complete updates
The service SHALL keep download and encoding workspaces outside the application installation directory that Setup replaces and the managed update staging directory that cleanup owns. Empty workspace configuration SHALL retain the existing protected data-directory default. Configuration and bootstrap Setup SHALL reject a workspace inside either reserved directory, including existing junction aliases, before replacing files.

#### Scenario: Workspace points into the installation
- **WHEN** configuration or bootstrap Setup encounters an installation or managed-update staging folder, or its descendant, as the download/encoding workspace
- **THEN** it rejects the unsafe path with an English explanation before replacing files, allowing the user to choose a separate workspace using the installed version

### Requirement: Desktop encoding pause mode
Desktop settings SHALL persist a pause mode with `ram` displayed as "RAM pause"; legacy empty or finish desktop values SHALL migrate to ram. A keyboard-accessible tooltip SHALL explain that RAM pause retains RAM/VRAM, resumes without restarting encoding, and cannot survive process exit, shutdown or reboot. Mode-only edits SHALL apply without restarting encodes.

#### Scenario: Choose RAM pause
- **WHEN** the user presses Pause encodes
- **THEN** both CPU and GPU profiles suspend active encoding processes and the dashboard identifies suspended jobs as paused in RAM

#### Scenario: Restore saved settings
- **WHEN** settings are loaded after a service restart
- **THEN** the selected mode is retained, with missing or legacy finish desktop values interpreted as ram

#### Scenario: Explain resource retention
- **WHEN** the user hovers over or focuses the pause-mode help control
- **THEN** the tooltip explains retained RAM and VRAM and the inability to preserve running encoding across process exit or reboot

#### Scenario: Change mode during pause
- **WHEN** the user changes only the pause mode while paused
- **THEN** the strategy is saved without changing either general pause preference

### Requirement: Per-encode pause controls
The dashboard SHALL retain its general control and show an individual Pause/Resume button for each controllable active CPU/GPU encoding. It SHALL explain that individual controls pause in RAM, preserve individual holds across general Resume, and identify individual versus general holds. The service SHALL route job IDs to their owning profile and reject mutations during maintenance or pending reconfiguration.

#### Scenario: Pause and resume a job row
- **WHEN** the user pauses or resumes an active encoding from its row
- **THEN** the action reaches that job's owning profile and updates that row's pause state without changing the other profile or its limits

#### Scenario: General pause holds a row
- **WHEN** the general RAM pause holds an encoding
- **THEN** the row explains the general hold and disables its Resume button until the general pause is lifted

#### Scenario: Queued or completed job
- **WHEN** a job has no active encoding process
- **THEN** no individual active-encode control is offered and a stale direct request is rejected

#### Scenario: Mandatory update or pending configuration
- **WHEN** maintenance or deferred reconfiguration is active and a UI or direct request attempts an individual pause
- **THEN** the mutation is blocked so accepted work can drain

### Requirement: Separate queue and encoding pause controls
The dashboard SHALL show Stop after current / Resume queue and Pause encodes / Resume encodes as separate controls, with equivalent tray actions. The former SHALL allow running encodes to finish; the latter SHALL immediately suspend them in RAM. Controls SHALL preserve each other's state and individual holds, remain locked during maintenance, and persist both desktop preferences independently.

#### Scenario: Both controls active
- **WHEN** the user stops after current and then pauses encodes
- **THEN** resuming encodes lets active work continue without reopening the queue; resuming the queue first leaves encoding suspended

#### Scenario: Restart with saved controls
- **WHEN** the service restarts with independently saved queue and RAM preferences
- **THEN** both are applied before restored encodes or admission can start, without claiming that live encoder progress survives restart

#### Scenario: Update draining
- **WHEN** maintenance drains a worker with both controls active
- **THEN** accepted work resumes and finishes, new assignments remain blocked and both saved preferences are preserved for the new installation

### Requirement: Explicit shutdown after accepted work
Windows Stop after current SHALL ask whether to shut down after accepted work. Only a positive answer SHALL arm a one-time, nonpersistent shutdown request. Both profiles SHALL finish accepted work including queued encodes and uploads before shutdown, with new admission closed. Ordinary pause, idle, scheduling and maintenance SHALL NOT arm shutdown.

#### Scenario: User chooses shutdown
- **WHEN** the user answers Yes to the Stop after current shutdown question
- **THEN** both profiles stop accepting work, finish accepted jobs and in-flight requests, and the service requests normal Windows shutdown once after both report idle

#### Scenario: User declines
- **WHEN** the user answers No
- **THEN** ordinary Stop after current applies without a shutdown request

#### Scenario: Cancel or lifecycle transition
- **WHEN** the queue is resumed, configuration or maintenance begins, a worker exits or cannot report status, or the service restarts
- **THEN** the shutdown request is canceled and cannot fire from a later idle state

#### Scenario: Pending shutdown and failed shutdown
- **WHEN** shutdown is armed or Windows rejects the shutdown request
- **THEN** the dashboard shows the pending action with Resume queue cancellation or the error, and no automatic repeat shutdown occurs

### Requirement: Graceful manual Windows upgrade wait
Manual Setup SHALL use its bundled helper to close CPU and GPU admission and wait for all accepted work, including queued encodes and uploads, before stopping the service or replacing files. It SHALL display remaining work, preserve settings and never force-stop jobs or arm PC shutdown. Waiting SHALL be bounded to 24 hours; errors or timeout SHALL leave files and active work intact.

#### Scenario: CPU is busy and GPU can accept work
- **WHEN** manual Setup starts with accepted CPU work and an available GPU profile
- **THEN** both profiles stop admission, accepted work finishes, and Setup continues automatically after both confirm idle

#### Scenario: Queued or RAM-paused accepted work
- **WHEN** Setup waits with queued or suspended accepted encodes
- **THEN** those encodes resume and finish including verification and upload while new requests remain blocked

#### Scenario: Upgrade from an older service
- **WHEN** the installed helper has the previous immediate-refusal behavior
- **THEN** the new installer uses its own helper and existing worker controls to wait for safe installation

#### Scenario: Unknown activity or competing update
- **WHEN** a configured profile cannot report its activity or a mandatory automatic update is pending
- **THEN** manual Setup reports the error without stopping the service or replacing files

#### Scenario: Wait timeout or cancellation
- **WHEN** the helper wait is canceled or exceeds 24 hours
- **THEN** Setup does not stop active work or replace files; any existing admission hold permits accepted work to finish and persisted settings remain unchanged
