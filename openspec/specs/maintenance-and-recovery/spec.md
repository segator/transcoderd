# Maintenance and recovery

## Purpose

Define the server's periodic handling of missing sources, timed-out assignments, recoverable failures, stale progress, and a worker's local restart recovery.

## Requirements

### Requirement: Periodic job maintenance
The server SHALL run job maintenance at `server.scheduler.scheduleTime`, whose registered default is five minutes. A maintenance pass SHALL inspect queued, failed, and assigned or started jobs.

#### Scenario: Missing queued source
- **WHEN** maintenance finds that a queued job's source file no longer exists
- **THEN** it appends `Job/failed` with a missing-source message

### Requirement: Assignment timeout
The server SHALL requeue an assigned or started job whose last job event is older than `server.scheduler.jobTimeout`, whose registered default is 24 hours. It SHALL record cancellation of the old assignment before the new queued event.

#### Scenario: Assignment exceeds timeout
- **WHEN** the last job event of an assigned or started job is older than the configured timeout
- **THEN** the server appends `Job/canceled` followed by `Job/queued`

### Requirement: Classified failure retry
The server SHALL requeue failed jobs only when their failure message matches its recoverable-error classification. After more than ten `Job/failed` events in a job's history, it SHALL cancel a recoverable job rather than retry it again.

#### Scenario: Recoverable failure
- **WHEN** a failed job has a classified recoverable message and has not exceeded the failure limit
- **THEN** it receives a new `Job/queued` event

#### Scenario: Excessive failures
- **WHEN** a recoverable job has more than ten failed job events
- **THEN** it receives `Job/canceled` with the too-many-attempts reason

#### Scenario: Unclassified failure
- **WHEN** the failure message is not classified as recoverable
- **THEN** maintenance does not automatically requeue the job

### Requirement: Stale progress cleanup
The server SHALL inspect progress entries approximately every minute and delete those whose last update is older than 24 hours.

#### Scenario: Abandoned progress
- **WHEN** a progress entry has not been updated for more than 24 hours
- **THEN** it is removed during the next progress-maintenance pass

### Requirement: Persist local worker context
The worker SHALL persist a JSON job context after publishing a task event, including the job ID, current event, source information, output information when present, and working directory.

#### Scenario: Phase event published
- **WHEN** the worker publishes a task event for an active job
- **THEN** the job context is written to its workspace for restart recovery

### Requirement: Resume local job contexts
At startup, the worker SHALL find JSON contexts in its workspace and enqueue each valid context at the phase identified by its last stored event.

#### Scenario: Context from earlier run
- **WHEN** the worker starts with a valid persisted job context
- **THEN** it counts the job as active and queues the recorded phase
