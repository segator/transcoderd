# Spec Delta

## MODIFIED Requirements

### Requirement: Runtime worker admission controls
A running worker SHALL provide local controls to report admission, job, and encode counts and set the job maximum to any non-negative integer without restart. A positive value SHALL update admission and the FFmpeg encode target. Zero SHALL stop requests and preserve the previous positive encode target. Lowering the target SHALL let active encodes finish before admitting another. Controls SHALL remain local.

#### Scenario: Read runtime admission status
- **WHEN** an operator runs the local status command
- **THEN** the worker reports admission state, active and reserved jobs, job maximum, active and target encodes, and schedule state

#### Scenario: Disable new job requests
- **WHEN** an operator sets the maximum to zero
- **THEN** the worker stops new requests while active and restored jobs continue, including their later FFmpeg phase

#### Scenario: A request is already in flight
- **WHEN** an operator sets the maximum to zero while a reserved request is waiting for the server
- **THEN** the worker reports that reservation and does not start further requests; the server may still assign the reserved job

#### Scenario: Change the maximum at runtime
- **WHEN** an operator raises the maximum from two to three and a third accepted job is waiting at FFmpeg
- **THEN** the worker starts a third concurrent FFmpeg job without restarting

#### Scenario: Lower the encode maximum without interruption
- **WHEN** an operator lowers the maximum below the number of active FFmpeg jobs
- **THEN** existing encodes continue and no additional FFmpeg job starts until the active encode count is below the new target

#### Scenario: Reject invalid maximum
- **WHEN** an operator supplies a negative or non-integer maximum
- **THEN** the command reports an error and leaves the current admission and encode limits unchanged

### Requirement: Bounded concurrent encoding
The worker SHALL bound FFmpeg jobs by target and preserve per-job workspace, progress, and outcome. Startup target SHALL use positive `worker.ffmpegConfig.parallelJobs`, else `worker.maxActiveJobs` with minimum one for restored work. Positive runtime `set-max-jobs` SHALL replace the target. Each non-FFmpeg phase SHALL have one consumer.

#### Scenario: Two encode jobs queued
- **WHEN** two accepted jobs reach the encode phase and the limit is `2`
- **THEN** both encoding jobs can run concurrently and each retains its own output and progress

#### Scenario: Encode limit reached
- **WHEN** the configured number of encoding jobs is already running
- **THEN** additional encode jobs wait until a slot is available

#### Scenario: Default encode concurrency follows admission
- **WHEN** `worker.maxActiveJobs` is `4` and the encode concurrency override is omitted
- **THEN** up to four accepted jobs can encode simultaneously

#### Scenario: Runtime encode target overrides startup value
- **WHEN** an operator sets a positive max-jobs value different from the startup encode target
- **THEN** the FFmpeg target changes to that value while each non-FFmpeg phase processes one job at a time
