## MODIFIED Requirements

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
- **THEN** running work finishes, prefetched encodes remain parked while the queue is stopped, new assignments remain blocked and both saved preferences are preserved for the new installation

### Requirement: Explicit shutdown after accepted work
Windows Stop after current SHALL ask whether to shut down after accepted work. Only a positive answer SHALL arm a one-time, nonpersistent shutdown request. Both profiles SHALL finish executing work including uploads before shutdown, with new admission closed and recoverable prefetched encodes parked. Ordinary pause, idle, scheduling and maintenance SHALL NOT arm shutdown.

#### Scenario: User chooses shutdown
- **WHEN** the user answers Yes to the Stop after current shutdown question
- **THEN** both profiles stop accepting work, finish executing jobs and in-flight requests, retain recoverable prefetched jobs without encoding them, and the service requests normal Windows shutdown once after both report safe idle

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
Manual Setup SHALL use its bundled helper to close CPU and GPU admission and wait for executing work and uploads to finish, retaining stopped queues' recoverable prefetched encodes, before stopping the service or replacing files. It SHALL display remaining work, preserve settings and never force-stop jobs or arm PC shutdown. Waiting SHALL be bounded to 24 hours; errors or timeout SHALL leave files and active work intact.

#### Scenario: CPU is busy and GPU can accept work
- **WHEN** manual Setup starts with accepted CPU work and an available GPU profile
- **THEN** both profiles stop admission, executing work finishes, and Setup continues automatically after both confirm safe idle

#### Scenario: Queued or RAM-paused accepted work
- **WHEN** Setup waits with prefetched encodes on a stopped queue or suspended running encodes
- **THEN** prefetched jobs remain parked, and suspended running encodes prevent safe idle until resumed and completed including verification and upload

#### Scenario: Upgrade from an older service
- **WHEN** the installed helper has the previous immediate-refusal behavior
- **THEN** the new installer uses its own helper and existing worker controls to wait for safe installation

#### Scenario: Unknown activity or competing update
- **WHEN** a configured profile cannot report its activity or a mandatory automatic update is pending
- **THEN** manual Setup reports the error without stopping the service or replacing files

#### Scenario: Wait timeout or cancellation
- **WHEN** the helper wait is canceled or exceeds 24 hours
- **THEN** Setup does not stop active work or replace files; any existing admission hold permits accepted work to finish and persisted settings remain unchanged

## ADDED Requirements

### Requirement: Protected Windows profile configuration
The desktop service SHALL support independently provisioned protected credentials and persistent workspace identities for its CPU and GPU profiles. Credentials SHALL remain private to the service and SHALL NOT appear in public status. The configured workspace marker SHALL be verified by each worker before protected admission or safe idle is enabled; an absent or different marker SHALL NOT be silently replaced.

#### Scenario: Both profiles are provisioned
- **WHEN** the operator provides a distinct server credential and matching persistent workspace identity for each profile
- **THEN** each child worker uses the corresponding credential and workspace identity, and public service status does not reveal either credential

#### Scenario: Provisioning is incomplete or storage changes
- **WHEN** a profile has an incomplete credential/identity pair or its expected workspace marker is absent or different
- **THEN** the service rejects the configuration or the profile refuses protected admission and cannot report safe idle

#### Scenario: Cancel a blocked configuration change
- **WHEN** stopped or suspended work prevents a configuration change from applying and the operator cancels the pending change
- **THEN** the service retains the existing configuration and releases its temporary admission hold without altering queue or encoding pauses
