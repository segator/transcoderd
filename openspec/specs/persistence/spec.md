# Persistence and status

## Purpose

Define the durable job, event, and worker-presence data required by the server, along with the temporary progress data used for monitoring.

## Requirements

### Requirement: Database migrations
At startup, the server SHALL create the schema-version record if needed, apply pending embedded SQL migrations in version order, and record each applied version.

#### Scenario: New database
- **WHEN** no schema version has been recorded
- **THEN** the server creates version tracking and applies available migrations

#### Scenario: Existing database
- **WHEN** a migration version is already recorded
- **THEN** the server does not apply that migration again

#### Scenario: Migration fails
- **WHEN** a pending migration fails
- **THEN** database initialization fails and the server does not start its scheduler and API

### Requirement: Durable job record
The server SHALL store each job's UUID, relative source and destination paths, source size, destination size, and associated event history.

#### Scenario: Eligible new source
- **WHEN** an eligible source has no existing job
- **THEN** the server stores its job record and initial `Job/queued` event

#### Scenario: Completed output
- **WHEN** completion processing finds the encoded destination
- **THEN** the persisted destination size reflects that file

### Requirement: Query job history and status
Internal server operations SHALL be able to load a job by UUID or source path with its ordered events and latest status, and SHALL be able to select jobs by current phase and state.

#### Scenario: Existing UUID
- **WHEN** the server loads a persisted job by UUID
- **THEN** it obtains the job data, event history, and latest projected status

#### Scenario: Queued selection
- **WHEN** the dispatcher selects the next queued job
- **THEN** it uses the persisted current status to find an eligible job

### Requirement: Project current job status
Each accepted notification SHALL update the job's current projected status so that scheduling and monitoring can use the latest phase, status, message, worker, and event time.

#### Scenario: Assignment recorded
- **WHEN** a `Job/assigned` event is inserted
- **THEN** the current projection identifies the assigned worker and state

#### Scenario: Failure recorded
- **WHEN** a `Job/failed` event is inserted
- **THEN** the current projection contains the failure state and message

### Requirement: Worker presence and temporary progress
The server SHALL update a worker's address and last-seen time by worker name. It SHALL retain active phase progress with percentage, ETA, worker, and update time until a final state or maintenance cleanup removes it.

#### Scenario: Repeated presence
- **WHEN** an existing worker sends another `Ping`
- **THEN** its address and last-seen time are updated

#### Scenario: Active phase progress
- **WHEN** a progressing event arrives
- **THEN** its current value is available in progress storage

### Requirement: Optional worker runtime telemetry
Worker presence SHALL retain optional runtime telemetry from Ping events, including encoder, CPU/GPU kind, admission and schedule state, active jobs, active encodes, and encode target. Presence freshness SHALL use server receipt time. Pings without telemetry SHALL remain accepted and indicate that runtime measurements are unavailable.

#### Scenario: Modern worker heartbeat
- **WHEN** a worker sends Ping with runtime status
- **THEN** the server updates presence and exposes that status to authenticated dashboard readers

#### Scenario: Legacy worker heartbeat
- **WHEN** a worker sends Ping without runtime status
- **THEN** presence still updates and the dashboard reports runtime measurements as unavailable

### Requirement: UTC server monitoring timestamps
New server-received job notifications, worker presence, and progress updates SHALL record UTC timestamps independent of the server or database session timezone. Dashboard time buckets and freshness SHALL use UTC instants. Existing naive historical timestamps SHALL remain unchanged because their original timezone cannot be inferred reliably.

#### Scenario: Server uses a non-UTC timezone
- **WHEN** a server configured with a non-UTC local timezone receives a job notification or presence update
- **THEN** a dashboard refresh reports the event at its actual receipt instant rather than shifting it by the local UTC offset

### Requirement: Atomic per-worker encoder limit pairs
The server SHALL persist optional non-negative `cpuJobs` and `gpuJobs` together by stable worker name. Incomplete pairs, negative values, mixed single/pair inputs, and integer-overflowing sums SHALL be rejected without mutation. Legacy writes SHALL clear the pair. Legacy clients without independent-encoder capability SHALL read aggregate zero when a pair exists.

#### Scenario: Pair survives server restart
- **WHEN** CPU 2 and GPU 3 are saved for a worker and read through a fresh repository
- **THEN** both values are restored together and another worker remains unchanged

#### Scenario: Invalid pair
- **WHEN** a request contains only one class limit, a negative value, or an overflowing sum
- **THEN** the server returns an input error and keeps the previously stored settings

#### Scenario: Old worker reads a dual override
- **WHEN** a client without the independent-encoder capability reads a stored pair
- **THEN** it receives aggregate zero and drains accepted work instead of assigning all slots to its only encoder
