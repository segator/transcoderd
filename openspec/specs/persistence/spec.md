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
