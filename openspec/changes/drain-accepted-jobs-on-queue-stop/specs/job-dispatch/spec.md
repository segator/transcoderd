## ADDED Requirements

### Requirement: Safely idle with prefetched jobs
When admission is stopped, the worker SHALL finish running encodes and their remaining phases while holding prefetched jobs before encoding. It SHALL distinguish running work, unresolved reservations and durably parked jobs. It SHALL report safe idle only with admission closed, no executing or suspended work, no unresolved requests and all parked jobs recoverable and acknowledged by the server.

#### Scenario: Active and prefetched jobs at queue stop
- **WHEN** the queue is stopped while an encode is running and a prefetched job awaits encoding
- **THEN** the running job finishes through upload, the prefetched job stays parked without starting FFmpeg, and the worker reports safe idle once the remaining conditions are satisfied

#### Scenario: Pre-encode preparation still running
- **WHEN** a previously accepted job is downloading or preparing subtitles as the queue is stopped
- **THEN** its preparation can finish, it parks before encoding, and the worker does not report safe idle while preparation or persistence is in progress

#### Scenario: In-flight request returns after stop
- **WHEN** a request reserved before queue stop returns an assignment
- **THEN** the worker retains the assigned job, keeps admission closed and cannot report safe idle until the request and any required preparation and parking finish

#### Scenario: Suspended running encode
- **WHEN** a running encode remains paused in RAM or by an individual hold
- **THEN** the worker does not report safe idle and does not silently resume or cancel the encode

#### Scenario: Unconfirmed parked work
- **WHEN** a prefetched job has missing, corrupt, unpersisted or unconfirmed recoverable state
- **THEN** the worker reports the problem and does not report safe idle

### Requirement: Resume parked prefetched jobs
On queue resume, the worker SHALL recover and start its parked prefetched jobs before requesting new assignments, subject to encoder capacity and independent holds. Queue stop and resume SHALL preserve job identity and assigned ownership.

#### Scenario: Resume a parked job
- **WHEN** an operator resumes a stopped queue with a durably parked prefetched job
- **THEN** that job becomes eligible to encode before the worker requests another job

#### Scenario: Restart with the queue stopped
- **WHEN** a worker restarts with a stopped queue and a valid parked prefetched job
- **THEN** it keeps the job parked and admits no new assignments until the queue is resumed

### Requirement: Local wait for safe restart
The worker SHALL provide a local operation to wait for safe idle under a restart barrier. The barrier SHALL prevent admission and job starts until restart or explicit release. The operation SHALL fail closed if worker status is unavailable; cancellation or timeout SHALL leave jobs and assignments intact.

#### Scenario: Parked work does not block restart
- **WHEN** an operator waits for a stopped worker that has no running work or reservations and has durably parked prefetched jobs
- **THEN** the operation reports success without starting the parked encodes

#### Scenario: Unknown or changing state
- **WHEN** the worker cannot confirm status, admission reopens or the wait times out
- **THEN** the operation does not report safe idle or cancel jobs

#### Scenario: Resume races with safe restart
- **WHEN** queue resume races with or follows a successful safe-idle wait
- **THEN** resume cannot admit or start work while the restart barrier remains held
