# Spec Delta

## MODIFIED Requirements

### Requirement: Local job admission limit
A worker SHALL admit at most its effective FFmpeg encode target plus one pipeline job, counting accepted jobs and in-flight reservations. Admission SHALL require a positive maximum and an open schedule. The default encode target SHALL remain two. Zero SHALL stop requests and new encodes; running encodes and downstream work SHALL finish. Lowering a positive target SHALL not interrupt active encodes.

#### Scenario: Ordinary active-job limit
- **WHEN** no time window is configured and three jobs are active or reserved at the default target
- **THEN** the worker does not request another job

#### Scenario: Custom active-job limit
- **WHEN** the effective encode target is `4` and four jobs are active
- **THEN** the worker can request another job

#### Scenario: Zero active-job limit
- **WHEN** the active-job maximum is set to zero at startup or runtime
- **THEN** the worker requests no new jobs and finishes running encodes and downstream work while retaining queued encodes

#### Scenario: Lower maximum while jobs are active
- **WHEN** an operator lowers the maximum below the number of active jobs
- **THEN** active jobs continue and the worker requests no jobs until capacity is available

#### Scenario: Raise maximum while worker is ready
- **WHEN** an operator raises the maximum above the active-job count
- **THEN** the worker may request jobs until the active-job count reaches the new encode target plus one

#### Scenario: Limit within configured window
- **WHEN** local time is inside a configured interval but active jobs and reservations have reached the encode target plus one
- **THEN** the worker does not request another job

#### Scenario: Outside configured window
- **WHEN** local time is outside the configured interval
- **THEN** the worker does not request a job

### Requirement: Runtime worker admission controls
Local controls SHALL persist operator limits before reporting success. Positive maxima SHALL set FFmpeg concurrency and resume queued encodes. Zero SHALL retain the positive target but stop requests and new encodes; running encodes and downstream work SHALL finish. Startup SHALL use server overrides for standalone workers and service-owned Windows limits. Maintenance draining SHALL finish accepted work without changing saved preferences. Controls SHALL remain local.

#### Scenario: Read runtime admission status
- **WHEN** an operator runs the local status command
- **THEN** the worker reports whether admission is enabled, active and reserved jobs, the job maximum, active and target encodes, and whether the schedule permits requests

#### Scenario: Disable new job requests
- **WHEN** an operator sets the maximum to zero
- **THEN** the worker stops new requests and lets active encodes, verification and uploads finish, and holds waiting FFmpeg jobs until a positive maximum resumes them

#### Scenario: A request is already in flight
- **WHEN** an operator sets the maximum to zero while a reserved request is waiting for the server
- **THEN** the worker reports that reservation and does not start further requests; the server may still assign the reserved job

#### Scenario: Change the maximum at runtime
- **WHEN** an operator sets a positive maximum different from the current value
- **THEN** the worker applies it to future job requests and FFmpeg encode concurrency without restarting

#### Scenario: Lower the encode maximum without interruption
- **WHEN** an operator lowers the maximum below the number of FFmpeg jobs already running
- **THEN** those encodes continue and no new encode starts until active encodes fall below the new target

#### Scenario: Reject invalid maximum
- **WHEN** an operator supplies a negative or non-integer maximum
- **THEN** the command reports an error and leaves the current maximum unchanged

#### Scenario: Paused encode slot becomes free
- **WHEN** an active encode finishes while paused and another accepted job is ready to encode
- **THEN** that queued job remains waiting until the user resumes

#### Scenario: Separate maintenance drain
- **WHEN** maintenance drains a paused worker for configuration or installation
- **THEN** all accepted work may finish while new assignments remain blocked and the saved pause preference remains intact

#### Scenario: Restore a persisted maximum after restart
- **WHEN** a standalone worker restarts with a persisted maximum for its configured worker name
- **THEN** it applies that value before requesting new jobs, even when its startup configuration specifies another maximum

#### Scenario: Use startup maximum when no override exists
- **WHEN** a standalone worker starts and the server has no persisted maximum for its configured worker name
- **THEN** it uses its configured startup maximum

#### Scenario: Server unavailable during persistence
- **WHEN** an operator changes the maximum while the server cannot persist the request
- **THEN** the local control reports an error and does not report the change as successful

## ADDED Requirements

### Requirement: Report pipeline admission capacity
Local worker status SHALL distinguish the configured encode target from the maximum number of accepted pipeline jobs.

#### Scenario: Read runtime admission status
- **WHEN** an operator runs the local status command
- **THEN** the worker reports the configured maximum and the accepted-job ceiling, along with active and reserved jobs
