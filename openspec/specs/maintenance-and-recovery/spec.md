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
The server SHALL requeue failed jobs only for an explicit recoverable cause, including transient network and RAM/VRAM allocation failures. Generic codec-probe warnings, encoder-parameter hints and source stream listings SHALL NOT alone qualify. Deterministic invalid subtitle or color failures SHALL take precedence over incidental warnings. After more than ten `Job/failed` events, it SHALL cancel a recoverable job rather than retry it again.

#### Scenario: Recoverable failure
- **WHEN** a failed job has a classified recoverable cause and has not exceeded the failure limit
- **THEN** it receives a new `Job/queued` event

#### Scenario: Excessive failures
- **WHEN** a recoverable job has more than ten failed job events
- **THEN** it receives `Job/canceled` with the too-many-attempts reason

#### Scenario: Unclassified failure
- **WHEN** the failure message is not classified as recoverable
- **THEN** maintenance does not automatically requeue the job

#### Scenario: Incidental codec warning
- **WHEN** a failed encode log contains a codec-probe warning or generic encoder-parameter hint without an explicit transient cause
- **THEN** maintenance does not requeue it based on that warning

#### Scenario: Deterministic subtitle or color error
- **WHEN** an unsupported subtitle, invalid subtitle input or invalid color configuration causes failure alongside probe warnings
- **THEN** maintenance retains the failure rather than blindly repeating it

#### Scenario: Resource allocation failure
- **WHEN** encoding explicitly fails to allocate RAM or VRAM without a deterministic input failure
- **THEN** maintenance can requeue it within the existing failure limit

### Requirement: Stale progress cleanup
The server SHALL inspect progress entries approximately every minute and delete those whose last update is older than 24 hours.

#### Scenario: Abandoned progress
- **WHEN** a progress entry has not been updated for more than 24 hours
- **THEN** it is removed during the next progress-maintenance pass

### Requirement: Persist local worker context
The worker SHALL persist a JSON job context including the job ID, current event, pending notification, source information, output information when present, and working directory. It SHALL atomically persist each pending notification before transmission and retain it until acknowledged.

#### Scenario: Phase event published
- **WHEN** the worker publishes a task event for an active job
- **THEN** the job context is written to its workspace for restart recovery

#### Scenario: A response is lost
- **WHEN** the server response to a notification is not received
- **THEN** the pending event remains available for an identical retry after restart

### Requirement: Resume local job contexts
At startup, the worker SHALL validate each recoverable JSON context against the server before counting it as active or enqueuing its recorded phase. The authenticated job lookup SHALL return HTTP 404 for a missing job. Pending notifications SHALL be reconciled before further phase events are created.

#### Scenario: Context from earlier run
- **WHEN** the server confirms the saved assignment is still started on the same worker
- **THEN** the worker counts it as active and queues the appropriate phase using the acknowledged event sequence

#### Scenario: Expired or missing assignment
- **WHEN** the job has been requeued, reassigned, ended, or removed
- **THEN** the worker discards the stale local context and its job workspace without publishing an event for that assignment

#### Scenario: Server temporarily unavailable
- **WHEN** the worker cannot determine the current assignment due to a lookup failure
- **THEN** it keeps the local context intact and retries the lookup until it succeeds or shutdown is requested

#### Scenario: Pending event after restart
- **WHEN** the worker restarts with an unacknowledged notification for its current assignment
- **THEN** it retransmits the identical event and records the acknowledgement before continuing
