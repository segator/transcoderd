## MODIFIED Requirements

### Requirement: Immediate Windows update admission barrier
After authenticating a complete eligible release, the service SHALL persist the pending update and immediately stop new job requests on both profiles. It SHALL download the installer while accepted work finishes and install as soon as the installer is verified and the profiles are safely idle. There SHALL be no confirmation or grace period. The admission block SHALL survive service and machine restarts until the installed release is verified current.

#### Scenario: Installer is still downloading
- **WHEN** a complete eligible release has been authenticated but its installer is still downloading
- **THEN** both profiles stop initiating job requests and continue only accepted work and requests already in flight

#### Scenario: Service restarts with a pending update
- **WHEN** the service starts with a persisted pending update
- **THEN** admission remains blocked before either worker profile can request new work and updating resumes automatically

### Requirement: Drain all accepted Windows work
Before replacing installation files, the service SHALL stop new assignments locally for both profiles and wait for safe idle. Active downloads, subtitle work, encodes, verification and uploads SHALL complete normally. When the queue was already stopped, recoverable prefetched jobs SHALL stay parked. An unreachable profile SHALL not be treated as idle. Maintenance SHALL preserve persisted server limits and user pause and schedule settings.

#### Scenario: Encode finishes while its upload is running
- **WHEN** encoding has completed but an accepted upload remains active
- **THEN** the installer is not started until that upload and all other accepted work finish

#### Scenario: Both profiles are busy
- **WHEN** CPU and GPU profiles have accepted work at update time
- **THEN** neither receives further assignments and installation waits for both profiles to become safely idle, with stopped queues' prefetched jobs retained

#### Scenario: Existing pause or schedule
- **WHEN** an update completes for a paused worker or outside its admission schedule
- **THEN** the service retains that pause or schedule and does not admit jobs contrary to it

#### Scenario: User paused before the update
- **WHEN** an authenticated mandatory update is detected while user pause holds prefetched encodes
- **THEN** maintenance finishes executing work without accepting new assignments or starting prefetched encodes, and the upgraded worker retains the saved pause and parked jobs
