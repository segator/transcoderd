## MODIFIED Requirements

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
