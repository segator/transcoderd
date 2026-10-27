# Spec Delta

## MODIFIED Requirements

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

## ADDED Requirements

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
