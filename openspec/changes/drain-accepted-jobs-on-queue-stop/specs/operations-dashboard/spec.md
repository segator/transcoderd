## MODIFIED Requirements

### Requirement: Live monitoring and individual capacity
The dashboard SHALL refresh periodically and show job phase progress and ETA when available, worker freshness, CPU/GPU encoders, active counts, and independent desired limits. Operators SHALL save CPU and GPU limits atomically per worker. Zero SHALL disable that class for new unassigned encodes; both zero SHALL pause admission. Running encodes and downstream work SHALL finish while prefetched encodes remain queued.

#### Scenario: Change one worker
- **WHEN** an authenticated operator saves CPU and GPU limits for one worker
- **THEN** the server persists that worker's pair, leaves other workers unchanged, and distinguishes desired from observed capacity until synchronization

#### Scenario: Missing or stale telemetry
- **WHEN** a worker has old heartbeat data or supplies no runtime telemetry
- **THEN** the dashboard labels its state and unavailable measurements rather than inventing CPU/GPU utilization

### Requirement: Observe worker UI changes and pause states
Worker heartbeats SHALL include observed queue/RAM holds, suspended encode count, individual job pause state, safe-idle state, executing and parked counts, and finishing/draining state when supported. The fleet dashboard SHALL distinguish these from offline, schedule and desired-limit acknowledgement states. Observed updates SHALL appear through its normal refresh without manually reloading. Paused encodes SHALL retain progress without a live-running ETA claim.

#### Scenario: Individual encode paused locally
- **WHEN** an operator pauses an active encode in the worker UI
- **THEN** the next heartbeat and fleet refresh identify that encode as paused without treating it as failed or completed

#### Scenario: RAM pause differs from queue stop
- **WHEN** an operator toggles RAM hold while positive limits remain configured
- **THEN** the dashboard identifies RAM pause and suspended encodes independently of the desired CPU/GPU values

#### Scenario: Remote change reaches local worker UI
- **WHEN** the fleet dashboard saves new limits and the worker synchronizes them
- **THEN** local worker controls and Windows profile status show the observed limits without restarting

### Requirement: Explicit acknowledged remote worker controls
Online capable workers SHALL offer Pause in RAM/Resume encodes separately from Stop after current/Resume queue. Commands SHALL require authenticated same-origin requests and worker acknowledgement, with pending/applied/failed feedback. Queue resume SHALL restore the last positive limits. RAM resume SHALL preserve queue and individual holds. Unsupported/offline/maintenance workers SHALL not accept these commands.

#### Scenario: Pause a running encode remotely
- **WHEN** an operator selects Pause in RAM
- **THEN** the worker suspends owned encodes through its local RAM control, retains limits and memory, acknowledges application, and the UI offers Resume encodes

#### Scenario: Stop and resume queue
- **WHEN** an operator selects Stop after current then Resume queue
- **THEN** active encodes finish while queued starts and admission stop, then the previous positive limits are restored without clearing independent RAM or individual holds

#### Scenario: Failed or superseded command
- **WHEN** a control fails or a newer command supersedes an unacknowledged command
- **THEN** the current command reports failure or pending accurately and an old acknowledgement cannot complete the newer command

#### Scenario: Prefetched work while safely idle
- **WHEN** a stopped worker reports durable parked jobs and no executing jobs
- **THEN** the dashboard distinguishes the prefetched count and assigned job ownership from safe-idle status and does not describe those jobs as completed

#### Scenario: Worker lacks safe-idle telemetry
- **WHEN** an older or offline worker does not report safe-idle state
- **THEN** the dashboard presents that state as unknown rather than inferring that restart is safe
