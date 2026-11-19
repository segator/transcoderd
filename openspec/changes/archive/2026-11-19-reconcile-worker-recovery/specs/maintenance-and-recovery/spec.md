## MODIFIED Requirements

### Requirement: Resume local job contexts
At startup, the worker SHALL validate each recoverable JSON context against the server before counting it as active or enqueuing its recorded phase. The authenticated job lookup SHALL return HTTP 404 for a missing job.

#### Scenario: Context from earlier run
- **WHEN** the server confirms the saved assignment is still started on the same worker
- **THEN** the worker counts it as active and queues the recorded phase using the server's latest event sequence

#### Scenario: Expired or missing assignment
- **WHEN** the job has been requeued, reassigned, ended, or removed
- **THEN** the worker discards the stale local context and its job workspace without publishing an event for that assignment

#### Scenario: Server temporarily unavailable
- **WHEN** the worker cannot determine the current assignment due to a lookup failure
- **THEN** it keeps the local context intact and retries the lookup until it succeeds or shutdown is requested
