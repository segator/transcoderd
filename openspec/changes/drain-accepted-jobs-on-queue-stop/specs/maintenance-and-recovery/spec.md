## MODIFIED Requirements

### Requirement: Assignment timeout
The server SHALL requeue an assigned or started job whose last job event is older than `server.scheduler.jobTimeout`, whose registered default is 24 hours, unless its current assignment has a confirmed parked state. It SHALL record cancellation of the old assignment before the new queued event.

#### Scenario: Assignment exceeds timeout
- **WHEN** the last job event of an assigned or started job is older than the configured timeout
- **THEN** the server appends `Job/canceled` followed by `Job/queued`

### Requirement: Persist local worker context
The worker SHALL persist a JSON job context after publishing a task event, including the job ID, current event, source information, output information when present, and working directory.

#### Scenario: Phase event published
- **WHEN** the worker publishes a task event for an active job
- **THEN** the job context is written to its workspace for restart recovery

### Requirement: Resume local job contexts
At startup, the worker SHALL find JSON contexts in its workspace and recover each valid context at its recorded recoverable next phase. Parked prefetched jobs SHALL remain parked while the queue is stopped.

#### Scenario: Context from earlier run
- **WHEN** the worker starts with a valid persisted job context
- **THEN** it preserves the accepted-job count and resumes the recorded phase according to admission and parked state

#### Scenario: Parked job survives assignment timeout
- **WHEN** a confirmed parked assignment exceeds the ordinary job timeout
- **THEN** the server retains its ownership without requeueing it and still permits explicit cancellation and recovery

#### Scenario: Restart after completed prefetch
- **WHEN** a valid parked job is recovered with its completed download and required files intact
- **THEN** it remains parked until queue resume and then continues to encoding without repeating that download

#### Scenario: Parked workspace is unavailable
- **WHEN** the saved context or downloaded data cannot be validated
- **THEN** the worker reports recovery failure, does not claim safe idle, and does not silently duplicate or complete the assignment

## ADDED Requirements

### Requirement: Durable parked assignment ownership
The server SHALL durably acknowledge a parked assignment and fence resume, cancellation and reassignment to prevent stale generations from changing authoritative state or publishing output. Worker and server SHALL reconcile interrupted park transitions. An explicit recovery path SHALL exist for permanently unavailable parked work.

#### Scenario: Lost park acknowledgement
- **WHEN** the worker loses an acknowledgement while parking a prefetched assignment
- **THEN** reconciliation establishes one authoritative ownership state before the worker reports safe idle

#### Scenario: Concurrent resume and reassignment
- **WHEN** resume races a cancellation or reassignment
- **THEN** stale ownership cannot update authoritative job state or publish an output
