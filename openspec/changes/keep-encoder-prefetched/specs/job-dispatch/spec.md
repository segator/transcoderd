# Spec Delta

## MODIFIED Requirements

### Requirement: Local job admission limit
A worker SHALL request a new job only when its active-job count plus in-flight reservations is below the effective FFmpeg encode target plus two bounded pipeline-buffer jobs, admission is enabled, and any configured time window permits work. The registered default target SHALL be two. Setting the maximum to zero at startup or runtime SHALL prevent new requests while active and restored jobs continue. Lowering the maximum SHALL affect future requests only.

#### Scenario: Ordinary active-job limit
- **WHEN** no time window is configured, two jobs are encoding at the default limit, and fewer than two additional jobs are active or reserved
- **THEN** the worker can request jobs to prepare upcoming encodes and overlap verification or upload with encoding

#### Scenario: Custom active-job limit
- **WHEN** the effective encode target is `4` and fewer than six jobs are active or reserved
- **THEN** the worker can request another job

#### Scenario: Zero active-job limit
- **WHEN** the active-job maximum is set to zero at startup or runtime
- **THEN** the worker requests no new jobs and continues active or restored jobs

#### Scenario: Lower maximum while jobs are active
- **WHEN** an operator lowers the encode target below the number of active jobs
- **THEN** active jobs continue and the worker requests no jobs until the active and reserved count is below the new encode target plus two

#### Scenario: Raise maximum while worker is ready
- **WHEN** an operator raises the encode target above the active-job count
- **THEN** the worker may request jobs until the active and reserved count reaches the new encode target plus two

#### Scenario: Limit within configured window
- **WHEN** local time is inside a configured interval but active jobs and reservations have reached the effective encode target plus two
- **THEN** the worker does not request another job

#### Scenario: Outside configured window
- **WHEN** local time is outside the configured interval
- **THEN** the worker does not request a job

### Requirement: Runtime worker admission controls
A running worker SHALL provide local controls to report admission, job, and encode counts and set the job maximum to any non-negative integer without restart. A positive value SHALL update the encode target and bounded admission ceiling. Zero SHALL stop requests and preserve the previous positive encode target. Lowering the target SHALL let active encodes finish and SHALL limit future admission to the new target plus the pipeline buffer. Controls SHALL remain local and SHALL NOT expose a remotely reachable management endpoint.

#### Scenario: Read runtime admission status
- **WHEN** an operator runs the local status command
- **THEN** the worker reports whether admission is enabled, active and reserved jobs, the job maximum, active and target encodes, and whether the schedule permits requests

#### Scenario: Disable new job requests
- **WHEN** an operator sets the maximum to zero
- **THEN** the worker stops new requests and leaves accepted and restored jobs running, including their later FFmpeg phase at the previous positive encode target

#### Scenario: A request is already in flight
- **WHEN** an operator sets the maximum to zero while a reserved request is waiting for the server
- **THEN** the worker reports that reservation and does not start further requests; the server may still assign the reserved job

#### Scenario: Change the maximum at runtime
- **WHEN** an operator sets a positive maximum different from the current value
- **THEN** the worker applies it to future job requests and FFmpeg encode concurrency without restarting

#### Scenario: Lower the encode maximum without interruption
- **WHEN** an operator lowers the maximum below the number of FFmpeg jobs already running
- **THEN** those encodes continue, no new encode starts until active encodes fall below the new target, and admission is bounded by the new target plus two

#### Scenario: Reject invalid maximum
- **WHEN** an operator supplies a negative or non-integer maximum
- **THEN** the command reports an error and leaves the current maximum unchanged

## ADDED Requirements

### Requirement: Report pipeline admission capacity
Local worker status SHALL distinguish the configured encode target from the maximum number of accepted pipeline jobs.

#### Scenario: Read runtime admission status
- **WHEN** an operator runs the local status command
- **THEN** the worker reports the configured maximum and the accepted-job ceiling, along with active and reserved jobs
